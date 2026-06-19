---
name: reproduce-first
description: "버그수정 red→green 하네스(분석/구현/평가 공유). 재현 테스트를 먼저 만들어 *실패(RED)* 시키고(=재현 증명), 수정 후 *통과(GREEN)* 시켜 닫는다. 재현 테스트는 정답 오라클 — 한 번 작성(analyzer), 불가침(implementer), 재실행으로 검증(evaluator). reproduce_unobtainable면 조기 중단."
type: skill
---

# Reproduce-First (red→green) — bugfix 검증 하네스

핵심 명제 (이 한 줄이 전부):
> **버그수정 = "재현 테스트가 RED(실패)였다가 GREEN(통과)이 되는 것."**
> RED를 못 보면 무엇을 고치는지 모르고, GREEN을 안 보면 고쳤는지 모른다.

**언제**: work_type=`bugfix` 의 모든 사이클(analyzer 재현 / implementer 수정 / evaluator 검증).
**아닐 때**: 새 기능(feature)은 재현할 버그가 없다 — 일반 TDD만(implementer §TDD).

## 재현 테스트 = 정답 오라클 (불변 규칙)

- **한 번 작성**: analyzer가 티켓의 "재현 방법" + root-cause로 **딱 하나** 만든다.
- **불가침**: implementer는 이 테스트를 **수정·삭제·약화하지 않는다**. 고쳐서 통과시키는 건
  *프로덕션 코드*이지 테스트가 아니다. (테스트를 손대 GREEN을 만드는 건 부정행위 = 거짓 GREEN.)
- **재사용**: bug 사이클이 돌아도 재생성하지 않는다. *잘못 작성된 경우에만* analyzer가 교정.
- **결정적**: 시간/난수/네트워크 의존 없이 항상 같은 결과.

## 세 게이트 (역할별)

```
RED  (analyzer §5):  재현 테스트 작성 → 실행 → 반드시 FAIL.
                     PASS면 = 재현 불가 → 1회 교정 → 그래도 PASS면
                     reproduction_unobtainable → 조기 BLOCKED(autonomy: escalate 1회).
                     기록: reproduction.json{ red_confirmed:true, red_output }, 마커 reproduction_confirmed.

CARRY (implementer): 재현 테스트를 **첫 커밋(test/red)**으로 올린다(수정 전). 그 다음 프로덕션
                     수정을 *수정용 단위테스트의* TDD(자체 red→green)로 완성. 재현 테스트는 불가침.
                     수정 후 로컬에서 재현 테스트를 돌려 GREEN인지 미리 확인(아니면 계속 수정).

GREEN (evaluator):   재현 테스트를 다시 실행 → 반드시 PASS(=문제 더 이상 재현 안 됨).
                     가능하면 parent/base 커밋에서도 돌려 FAIL을 재확인 → 진짜 red→green 증명.
                     PASS 못 하면 EVALUATION_FAIL: 실패문서 작성 → analyzer 재진입.
                     기록: reproduction.json{ green_confirmed, green_at_head, red_at_parent? }.
```

## reproduction.json (세 에이전트가 공유하는 계약)

```jsonc
{ "test_file": "<path>", "test_name": "<TestName>", "package": "<pkg>",
  "run_cmd": "go test -run '<TestName>' ./<pkg>/...", "race": false,
  "red_confirmed": true,  "red_output": "<failure tail>",   "authored_cycle": 1,
  "green_confirmed": null, "green_at_head": null, "red_at_parent": null }
```
analyzer가 RED 필드를, evaluator가 GREEN 필드를 채운다. implementer는 읽기만(+ 커밋).

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
