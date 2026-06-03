---
name: stablenet-invariants
description: "go-stablenet의 항상-켜진 byzantine-fairness 핵심 불변식(L3 backstop). 검색 품질과 무관하게 합의 안전성·공정성 판단의 기준선을 제공한다. Planner는 설계가 이를 위반하지 않도록, Evaluator는 diff가 이를 깨지 않았는지 판정하는 데 쓴다."
type: skill
---

# StableNet Critical Invariants (always-on backstop — L3)

이 불변식들은 cks 검색이 무엇을 surface 하든 **항상** 성립한다. 인덱스가
없거나 검색이 비어도 적용된다. 아래 중 하나라도 위반하면 테스트가 통과하더라도
byzantine-fairness 또는 합의 안전성 버그다. Planner는 설계 시, Evaluator는
diff 판정 시 이 목록을 기준으로 본다.

1. **EQUAL POWER.** 모든 StableNet validator의 의결권 = 1 (PoA 등가-power,
   WEMIX식 stake-weighted 아님). stake 가중 투표·보상·정족수 계산을 절대
   도입하지 말 것. 정족수 `Quorum = ⌈N − (N−1)/3⌉` 를 등가-power validator
   위에서 계산한다.

2. **EPOCH-LENGTH ASYMMETRY.** epoch 길이 변경은 validator 간 diligence/accounting
   에 **비대칭적으로** 영향을 준다. epoch 길이 변경은 단순 상수 수정이 아니라
   byzantine-fairness 변경이므로 명시적 공정성 분석을 요구한다.

3. **ROUND-CHANGE NEUTRALITY.** round change(타임아웃 시 proposer 회전)는
   proposer-share 나 보상 accounting 을 바꾸면 **안 된다**. round change 는
   liveness 메커니즘이지 경제적 이벤트가 아니다.

4. **QUORUM FLOAT SAFETY.** 정족수/임계값 계산은 정수/⌈⌉-정확이어야 한다.
   정족수에 float 비교를 쓰지 말 것 — float 정밀도가 표결 결과를 뒤집을 수 있다.

5. **STICKY-PROPOSER CONCENTRATION.** Sticky proposer 정책은 제안권을 집중시킨다.
   proposer 선정 변경은 validator 집합 전체에 대한 장기 공정성을 보존해야 한다
   (RoundRobin 과 Sticky 는 공정성 프로파일이 다르다).

---

합의 엔진 = **WBFT** (QBFT 계열, RPC namespace = `istanbul`). System contract 의
정식 이름은 `gov_council` / `gov_minter` / `gov_validator` 이며, WEMIX식 staking
계열 이름을 쓰지 않는다. 정확한 수치·anchor 는 cks `verified` 도메인 엔트리가
권위이며, 이 backstop 은 인덱스에 의존하지 않는 고정 기준선이다 — 둘이 어긋나면
엔트리를 따른다.
