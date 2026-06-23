---
name: reproduce-first
description: "버그수정 red→green 하네스(분석/구현/평가 공유). 재현 테스트를 먼저 만들어 *실패(RED)* 시키고(=재현 증명), 수정 후 *통과(GREEN)* 시켜 닫는다. 두 티어: simulation(go-stablenet 인-프로세스 Go 테스트) / e2e(프로젝트-빌드 바이너리로 chainbench 멀티노드, tests/repro/*.sh 누적). 재현 테스트는 정답 오라클 — 한 번 작성(analyzer), 불가침(implementer), 재실행으로 검증(evaluator). reproduce_unobtainable면 조기 중단."
type: skill
---

# Reproduce-First (red→green) — bugfix 검증 하네스

핵심 명제 (이 한 줄이 전부):
> **버그수정 = "재현 테스트가 RED(실패)였다가 GREEN(통과)이 되는 것."**
> RED를 못 보면 무엇을 고치는지 모르고, GREEN을 안 보면 고쳤는지 모른다.

> ⚠️ **재현 GREEN은 필요조건이지 충분조건이 아니다.** 재현 테스트(필수)는 "증상이 멈췄다"만
> 증명한다 — 수정이 *타당*한지(근본원인 edge를 고쳤나·형제 경로를 다 덮었나·회귀/오버핏은 없나)는
> **별개 판정**이다. 이 둘을 섞지 말 것:
> - **재현 판정(필수)** = 이 스킬의 RED→GREEN 오라클. (evaluator §4.7)
> - **수정 타당성 판정(충분)** = GREEN 위에서 추가 검증. (evaluator §4.8, analyzer §4.1 affected_sites)
> GREEN인데 타당성 FAIL이면 "버그 안 고쳐짐"이 아니라 "수정이 부당/불완전" — 재진입 안내가 다르다.

**언제**: work_type=`bugfix` 의 모든 사이클(analyzer 재현 / implementer 수정 / evaluator 검증).
**아닐 때**: 새 기능(feature)은 재현할 버그가 없다 — 일반 TDD만(implementer §TDD).

## 두 단계의 테스트 티어 (tier)

재현은 **증상을 가장 싸게 잡는 티어**에서 한다. 한 버그의 오라클은 **티어 하나**다.

- **tier=`simulation`** — 정적/인-프로세스 테스트. 대상 프로젝트(go-stablenet) 트리 안의
  Go 테스트(시뮬레이션 백엔드/합의 하네스). 바이너리·노드 불필요, 빠르고 결정적.
  *대부분의 버그*는 여기서 재현된다 — **simulation-first**.
- **tier=`e2e`** — 대상 프로젝트를 빌드한 **바이너리**로 chainbench가 로컬(혹은 remote)에
  멀티노드를 띄워 합의로 블록을 생성하는 환경에서 재현. chainbench `tests/<category>/<name>.sh`
  bash 테스트로 작성. 사전조건(컨트랙트 배포·계정 펀딩·tx 주입 등)을 구성해야 할 수 있다.
  **합의/동기화/P2P/txpool 전파/멀티노드 타이밍/하드포크처럼 단일 프로세스로는 재현되지 않는
  증상**, 또는 simulation 티어로 재현 실패 시 이 티어로 격상한다.

티어 선택은 analyzer §5가 한다(simulation 시도 → 못 잡거나 본질상 멀티노드면 e2e 격상).
선택된 티어가 그 버그의 오라클이고, 세 게이트(RED/CARRY/GREEN)는 그 티어에 맞춰 동작한다.

## 재현 테스트 = 정답 오라클 (불변 규칙)

- **한 번 작성**: analyzer가 티켓의 "재현 방법" + root-cause로 **딱 하나** 만든다.
- **불가침**: implementer는 이 테스트를 **수정·삭제·약화하지 않는다**. 고쳐서 통과시키는 건
  *프로덕션 코드*이지 테스트가 아니다. (테스트를 손대 GREEN을 만드는 건 부정행위 = 거짓 GREEN.)
- **재사용**: bug 사이클이 돌아도 재생성하지 않는다. *잘못 작성된 경우에만* analyzer가 교정.
- **결정적**: 시간/난수/네트워크 의존 없이 항상 같은 결과.

## 세 게이트 (역할별)

```
RED  (analyzer §5):  티어 선택 → 재현 테스트 작성 → 실행 → 반드시 FAIL.
   simulation:        go test 작성·실행, FAIL이어야 재현 확인.
   e2e:               대상 프로젝트를 **현재(미수정) 트리**로 빌드 → chainbench_init(
                      {profile, project_root, binary_path}) → chainbench_start → 사전조건
                      구성(contract_deploy/tx_send 등) → chainbench_test_run <repro> → FAIL.
                      .sh 테스트는 chainbench tests/repro/ 아래에 작성(= point 5 누적).
                     PASS면 = 재현 불가 → 1회 교정(또는 티어 격상) → 그래도 PASS면
                     reproduction_unobtainable → 조기 BLOCKED(autonomy: escalate 1회).
                     기록: reproduction.json{ tier, red_confirmed:true, red_output }, 마커 reproduction_confirmed.

CARRY (implementer): 재현 테스트는 **불가침**(수정·삭제·약화 금지). 고치는 건 프로덕션 코드뿐.
   simulation:        그 Go 테스트를 **첫 커밋(test/red)**으로 go-stablenet 트리에 올린다(수정 전).
   e2e:               오라클 .sh는 *chainbench 저장소*에 산다 — fix PR에 포함되지 않는다. 커밋 대상이
                      아니며, implementer는 절대 수정하지 않는다(reproduction.json이 참조).
                     수정 후 로컬에서 재현 오라클을 돌려 GREEN인지 미리 확인(아니면 계속 수정).

GREEN (evaluator):   재현 오라클을 다시 실행 → 반드시 PASS(=문제 더 이상 재현 안 됨).
   simulation:        HEAD에서 go test → PASS, repro 커밋에서 → FAIL(진짜 red→green), test_file diff 비어야.
   e2e:               HEAD(수정본)으로 바이너리 재빌드 → chainbench drive → repro 테스트 PASS;
                      parent/base 커밋으로 재빌드 시 FAIL 재확인; .sh 오라클 미변경 확인.
                     PASS 못 하면 EVALUATION_FAIL: 실패문서 작성 → analyzer 재진입.
                     기록: reproduction.json{ green_confirmed, green_at_head, red_at_parent? }.
```

## reproduction.json (세 에이전트가 공유하는 계약)

`tier`가 분기 키다. 공통 필드 + 티어별 필드:

```jsonc
{ "tier": "simulation",                 // "simulation" | "e2e"
  "test_name": "<TestName 또는 repro id>",
  "red_confirmed": true,  "red_output": "<failure tail>",   "authored_cycle": 1,
  "green_confirmed": null, "green_at_head": null, "red_at_parent": null,

  // tier == "simulation" (Go 인-프로세스, go-stablenet 트리):
  "test_file": "<go-stablenet 내 경로>", "package": "<pkg>",
  "run_cmd": "go test -run '<TestName>' ./<pkg>/...", "race": false,

  // tier == "e2e" (chainbench bash, chainbench 저장소):
  "chainbench_test": "repro/<ticket>-<slug>",                  // chainbench_test_run 인자(category/name)
  "chainbench_test_file": "<CHAINBENCH_DIR>/tests/repro/<ticket>-<slug>.sh",
  "profile": "regression",                                     // chainbench_init 프로필
  "binary_build_cmd": "make gstable",                          // 대상 프로젝트 빌드 명령
  "preconditions": ["deploy SimpleStorage", "..."] }           // 환경 사전조건 요약
```
analyzer가 `tier`+RED 필드를, evaluator가 GREEN 필드를 채운다. implementer는 읽기만(+ simulation은 커밋).

## loop-until-green (bug 사이클)

evaluator가 GREEN 못 보면 → 실패문서 → analyzer가 "무엇을 놓쳤나" 재분석(재현 테스트 **재사용**) →
planner plan-fix → implementer 재수정 → evaluator 재검증. `max_eval_cycles`까지. 재현 테스트는
변하지 않으므로 **같은 잣대로** 수렴 여부를 측정한다.

## 워크드 예 — txpool fee-payer drift (압축)

증상: 풀 포화 시 정상 fee-delegation tx가 부당 거부.
- RED(analyzer): `TestReproduce_FeePayerDriftRejectsValidTx` — 풀을 채우고 퇴출 유도 후 정상 FD tx
  전송이 거부됨을 assert. 현재 코드에서 **FAIL**(거부됨) = 재현 확인.
- CARRY(implementer): 위 테스트를 먼저 커밋 → truncatePending 퇴출 경로에 fee-payer 채무 해제 추가
  (수정용 단위테스트 TDD) → 재현 테스트 로컬 GREEN 확인.
- GREEN(evaluator): 재현 테스트 재실행 → **PASS**(정상 tx 수락). parent 커밋에선 여전히 FAIL → red→green 증명.

---
한 줄 트리거: **재현 테스트로 RED부터 보고 → 그 테스트는 건드리지 말고 코드로 GREEN을 만든다 → 재실행으로 닫는다.**
