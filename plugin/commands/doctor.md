---
description: 플러그인·프로젝트 환경 진단(read-only). 버전·env·MCP·cks·도메인팩 상태를 한 화면으로 보고하고, 빠진 것·재시작 필요·불일치를 알려준다. 아무것도 수정하지 않는다.
argument-hint: "[--project <id>] [--json]"
---

# /coding-agent:doctor

읽기 전용 환경 진단. **아무것도 쓰지 않는다**(수정은 `/coding-agent:setup`). 결정론 진단(스크립트)
+ 라이브 MCP 프로브를 합쳐 **READY / ATTENTION** 과 다음 행동을 보고한다.

---

## 1. 결정론 진단 (스크립트)

```
bash: python3 ${CLAUDE_PLUGIN_ROOT}/scripts/doctor.py --plugin-root "${CLAUDE_PLUGIN_ROOT}" {--project <id> 인자 있으면}
```

보고: 활성 플러그인 버전, 프로젝트/repo(`git rev-parse`), project_id+사용가능 팩, `repo_root_env`,
env(process + `.claude/settings*.json`, 시크릿 마스킹, **restart_needed**=settings엔 있으나 현재 env엔
없음), CKS_CONFIG 존재, permissions/allowlist. 이 출력을 사용자 보고에 그대로 포함한다.

## 2. 라이브 MCP 프로브 (이 명령이 직접 — §1 스크립트로는 못 보는 실연결)

cks 도구를 로드(미인식이면 ToolSearch 1회)한 뒤:

- `cks_ops_health` → status/serviceable, ckg·ckv reachable(+model), **source_root**, indexed_head, data_path.
- `cks_ops_freshness` → indexed_head vs current_head, changed_files.
- **정합 교차(핵심)**:
  - `source_root == §1 repo_root` 인가? 다르면 **⚠ 다른 체크아웃을 인덱싱** — 검색이 엉뚱한 트리 반영.
  - `indexed_head == 현재 HEAD` 인가? 아니면 stale(단, *의도된 base 인덱스*일 수 있음 — 판단만, 재인덱싱 금지).
- chainbench: `chainbench_status`(또는 config 조회)로 연결/프로파일 확인. 미연결이면 보고(SKIPPED, 차단 아님).
- jira(`requirement_source != "local"` 일 때만): 가벼운 호출로 도달 확인. 미연결이면 보고.

각 MCP가 미등록/미연결이어도 **차단하지 않고 사실만 보고**한다.

## 3. 판정 + 다음 행동

READY / ATTENTION 으로 종합하고, §1 issues + restart_needed + §2 MCP 미연결·불일치를 한 목록으로:

- `repo_root_env` unset/restart_needed → **`/coding-agent:setup --fix` 후 세션 재시작**.
- cks not serviceable → ckv/Ollama 기동 또는 `CKS_CONFIG` 재배선 + 세션 재시작.
- `source_root ≠ repo_root` → cks config 재설정 + 세션 재시작(현재 repo를 인덱싱하도록).
- index stale → 재인덱싱(⚠ *의도된 base 인덱스면 하지 말 것* — 사용자에게 확인).
- permissions 미등록이고 무인 실행 원하면 → `/coding-agent:setup --autonomous`.

**읽기 전용 계약**: doctor는 어떤 파일·설정·인덱스도 수정하지 않는다.
