---
name: perspective-reviewer
description: DESIGN 후 FREEZE 전, 변경에 적용되는 도메인 렌즈별로 설계를 적대적 교차검증한다. 실행 가능 게이트(chainbench)가 못 잡는 partial-binding의 distinguishing claim을 잡는 멀티뷰포인트 관문. blocking VIOLATION ≥1이면 설계 반려·revise.
---

# perspective-reviewer

설계(design)를 *도메인 렌즈*별로 적대적으로 심문한다. 단일 planner의 단일 관점을 깨고,
**불변식 게이트(#4)가 검증 못 하는 distinguishing claim**(예: base-fee 재분배-not-burned,
no-reorg, power-equality)을 LLM 적대 판단으로 메운다.

> 근거: `02-constraint-first-and-perspective-review.md`(perspective review), GLOSSARY §5(6 렌즈),
> §6 파이프라인 `DESIGN → PERSPECTIVE-REVIEW → FREEZE`.
> 실증(#5): chainbench 게이트는 partial-binding의 distinguishing claim을 못 잡음
> (c-03은 baseFee 소각해도 통과). 그 갭을 이 스킬이 담당.

## 입력
- `constraints.md` (constraint-assembler #3 산출 — 적용 불변식 + 각 lens + partial-binding flag).
- 설계 산출물 (design.md / 제안 diff).

## 절차

### 1. 렌즈 선택 (선택 활성화 — 전부 아님)
`constraints.md`의 적용 불변식들의 **distinct `lens` 집합**만 활성화(GLOSSARY §5 선택 활성화).
비용 폭발 방지(06 B1): 변경과 무관한 렌즈는 돌리지 않는다.
예) consensus/wbft 변경 → `fee-policy` · `consensus-safety` · `ledger-conservation` · `byzantine-fairness` 정도.

### 2. 렌즈별 적대 심문
각 활성 렌즈에 대해 `lenses.yaml`의 `ask` 질문 + `distinguishing_claims`로 설계를 심문한다.
**REFUTE 자세**: "이 설계가 이 불변식을 *어떻게 깰 수 있는가*"를 먼저 찾는다.
- **partial-binding(`gate_blind` 표시) 불변식을 우선 타깃** — 게이트가 통과시켜도 여기서 잡아야 함.
- 각 판정에 근거(인용: 불변식 id + 설계의 해당 부분 + 코드앵커).

### 3. 판정 분류
- `VIOLATION` (blocking): 설계가 불변식을 깬다/보증 못 한다 → **설계 반려**.
- `RISK` (non-blocking): 잠재 위험, 논증/테스트 보강 권고.
- `OK`: 렌즈가 보증 확인.

### 4. `perspective-report.md` 산출
```markdown
# perspective-report — <ticket-id>
활성 렌즈: <selected lenses>  (전체 6 중 N개)

## <lens> 렌즈
- [VIOLATION|RISK|OK] <불변식 id>: <판정 근거 + 인용>
- ...

## 종합
- blocking VIOLATION: <count>
- 결과: <REJECT(revise 필요) | PASS(freeze 허용)>
- gate-blind 커버: <게이트가 못 잡지만 이 렌즈가 검증한 distinguishing claim 목록>
```

## 출력 계약
- **blocking VIOLATION ≥1 → REJECT** → DESIGN으로 revise(≤K회, GLOSSARY §6). cap 도달 시 BLOCKED 보고.
- 활성 렌즈는 반드시 `constraints.md`의 lens 집합을 *모두* 덮어야 함(누락 금지).
- partial-binding 불변식은 각각 VIOLATION/RISK/OK 중 하나로 *명시 판정*(미판정 금지) — 게이트가 못 잡으므로 여기서 반드시 결론.

## 한계 (정직)
- LLM 적대 판단이라 false-positive(환각 VIOLATION) 가능 → revise cap(≤K) + 근거 인용 강제로 완화. cap 도달+미해결 VIOLATION이면 자동 통과 금지, **사람 게이트로 BLOCKED 보고**.
- 게이트(#4, 기계)와 *상보*: 기계는 bound 위반을, 렌즈는 distinguishing claim을 담당. 둘 다 통과해야 freeze.
