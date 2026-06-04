---
name: bench-orchestration
description: "동일 go-stablenet 태스크를 A(cks)/B(code-only)/C(code+skills) 3모드로 자율 실행하고 토큰·비용·정확성·안전성을 비교하는 harness-engineering 오케스트레이션. token limit 고려 배치+checkpoint/resume. /coding-agent:bench 가 호출."
type: skill
---

# Bench Orchestration (3-way comparison automation)

이 skill은 harness-engineering automation의 코어다. 하나의 실험 manifest(태스크 ×
모드)를 받아, **같은 태스크를 세 정보 regime으로 자율 실행**하고 결과를 비교한다:

- **A_cks** — 실제 `planner`(cks: semantic+graph+domain) → 공유 implementer → evaluator
- **B_code_only** — `bench-planner-codeonly`(grep/read만) → 공유 implementer → evaluator
- **C_code_skills** — `bench-planner-skills`(grep/read + 이해 skill) → 공유 implementer → evaluator

세 모드 모두 모델은 `claude-opus-4-7`(planner tier)로 고정 — 비교는 *정보 regime*을
격리한다. 측정은 결정적 tool(`bench/compare.py`)이, 실행 구동은 이 skill(skill 중심)
+ Agent tool(별도 mode-variant agent) + transcript hook이 담당한다.

> **Token-limit 인지.** plugin-native 실행은 사용자의 현재 세션 한도 안에서 돈다.
> 그래서 이 skill은 **한 번에 batch_size 셀만** 실행하고 checkpoint 후 멈춘다.
> 이어서 `/coding-agent:bench <experiment> --continue` 로 재개한다. 절대 한 번에
> 전체 매트릭스를 돌리지 않는다.

---

## 1. 실험 레이아웃 (state + checkpoint 계약)

```
.coding-agent/bench/{experiment}/
├── manifest.json          # 실험 정의(아래 §2)를 복사
├── state.json             # 셀 상태(아래 §3) — resume의 단일 진실
├── cells/
│   └── {task}__{mode}/    # 셀별 실행 워크스페이스 = 일반 ticket 워크스페이스 + run-meta.json
│       ├── run-meta.json  # { experiment, task, mode, mode_agent, started_at, ended_at }
│       ├── state.json     # 파이프라인 state (state-machine)
│       ├── logs/agent-transcript.jsonl   # transcript hook가 기록(P4) → 토큰 회계 원천
│       ├── analysis.md / plan.md / design-v*.md / test-report.md
│       └── ...
└── report/                # bench/compare.py 산출물(md/json/csv)
```

## 2. Manifest (입력)

`bench/manifests/*.json` (스키마: `bench/manifest.schema.json`):

```jsonc
{
  "experiment": "gsn-retrieval-abc-2026-06",
  "modes": ["A_cks", "B_code_only", "C_code_skills"],
  "mode_agents": {                       // 모드 → ANALYSIS/PLANNING/DESIGN agent
    "A_cks":         "planner",
    "B_code_only":   "bench-planner-codeonly",
    "C_code_skills": "bench-planner-skills"
  },
  "shared_agents": { "implement": "implementer", "evaluate": "evaluator" },
  "tasks": [
    { "id": "STABLE-0001",
      "ticket": "bench/fixtures/tickets/STABLE-0001.json",
      "oracle": { "chainbench_test": "basic/consensus" } }
  ],
  "go_stablenet_root": "/abs/path/to/go-stablenet",
  "batch_size": 1                        // 한 invocation당 실행할 셀 수(token 한계)
}
```

셀 집합 = `tasks × modes` (예: 1 task × 3 modes = 3 cells).

## 3. state.json (재개의 단일 진실)

```jsonc
{
  "experiment": "...",
  "created_at": "...", "updated_at": "...",
  "cells": [
    { "task": "STABLE-0001", "mode": "A_cks", "workspace": "cells/STABLE-0001__A_cks",
      "status": "pending|running|done|failed",
      "pipeline_state": "EVALUATION_PASS|...", "failure": null }
  ]
}
```

## 4. 프로토콜 (이 skill이 실행하는 절차)

```
4.1 진입(/coding-agent:bench)
    - 신규: manifest 경로 → .coding-agent/bench/{experiment}/ 생성, manifest 복사,
      state.json 초기화(모든 셀 status=pending).
    - --continue: 기존 experiment 디렉터리 로드.

4.2 MCP pre-flight (orchestrator §2.0 재사용)
    - A_cks 모드가 매트릭스에 있으면 cks/jira/chainbench 등록+env를 config-레벨로 확인.
    - B/C 모드만 있으면 cks 없이 진행 가능.

4.3 배치 선택(token-limit 인지)
    - pending 셀에서 앞에서부터 batch_size개 선택.

4.4 각 셀 실행(셀 = 한 (task, mode))
    a. cells/{task}__{mode}/ 생성, run-meta.json 기록(mode, mode_agent, 시작시각).
       state.json 셀 status=running.
    b. ticket을 셀 워크스페이스에 복사(manifest.tasks[].ticket → ticket.json),
       state-machine.init_state 로 파이프라인 state 초기화.
    c. ANALYSIS/PLANNING/DESIGN: Agent tool 로 mode_agents[mode] 디스패치
       (A=planner, B=bench-planner-codeonly, C=bench-planner-skills).
       → 동일 artifact(analysis.md, related-code.json, plan.md, design-v*.md) 생성.
    d. IMPLEMENTATION: Agent tool 로 shared_agents.implement(implementer) 디스패치
       → build/bin/gstable + state.json.binary_path.
    e. EVALUATION: Agent tool 로 shared_agents.evaluate(evaluator) 디스패치
       → test-report.md + chainbench summary.failed.
    f. transcript hook(P4)가 각 sub-agent 디스패치의 prompt/response를 셀 워크스페이스
       logs/agent-transcript.jsonl 에 자동 기록(토큰 회계 원천).
    g. state.json 셀 status=done(또는 failed + 사유), ended_at 기록.

    각 셀은 독립 — 한 셀 실패가 다음 셀을 막지 않는다(기록 후 계속).

4.5 측정 + 리포트(배치 후, 또는 모든 셀 done 시)
    Bash: python3 bench/compare.py \
            --experiment-dir .coding-agent/bench/{experiment} \
            [--prices bench/prices.json]
    → report/{comparison.md, comparison.json, comparison.csv} 생성.
    md 요약 표(모드 × {정확성, 토큰, 비용, 지연, 안전성})를 사용자에게 출력.

4.6 진행 보고 + 재개 안내
    - pending 셀이 남았으면: "남은 N 셀. 이어서: /coding-agent:bench {experiment} --continue"
    - 모두 done: 최종 리포트 경로 안내.
```

## 5. 안전/경계 정책
- 이 skill은 벤치 전용. 셀 워크스페이스는 `.coding-agent/bench/`(일반 `/work`의
  `.coding-agent/tickets/`와 분리)에 격리.
- 측정 tool은 결정적(LLM 없음)·read-only — 셀 산출물을 읽기만 한다.
- B/C 모드 agent는 cks tool grant가 없어(별도 agent) regime이 하드하게 분리된다 —
  "cks 쓰지 마"라는 프롬프트 의존이 아니라 도구 부재로 보장.
- 실패한 셀도 리포트에 포함(정확성=실패로 집계) — 누락 truncation 금지.
