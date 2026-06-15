---
name: constraint-assembler
description: DESIGN 직전, 변경 대상 모듈에 적용되는 불변식·규칙을 인용과 함께 constraints.md로 산출한다. "그럴듯한 오답 밀어붙이기"를 막는 제약-우선(constraint-first) 게이트. 버그픽스/기능 구현의 ANALYSIS→PLANNING 이후 DESIGN 전에 호출.
---

# constraint-assembler

설계를 *쓰기 전에* "이 변경에 적용되는 제약"을 **명시 산출물**(`constraints.md`)로 먼저 뽑는다.
제약을 LLM의 *회상*이 아니라 *인용 가능한 목록*으로 고정해, 설계가 제약을 만족하도록 강제한다.

> 근거: `02-constraint-first-and-perspective-review.md`(제약 우선), D-007(P0 = degraded),
> GLOSSARY §6 파이프라인 `ANALYSIS → CONSTRAINT-ASSEMBLY → DESIGN`.
> 실증: bench C셀이 `stablenet-invariants` backstop으로 INV-7(base-fee 재분배)을 잡아
> `return nil` 오답을 회피한 반면, code-only(B)는 추론에 그쳤다. 이 스킬은 그 catch를
> *스킬 운*이 아니라 *결정적 산출물*로 만든다.

## 입력
- 티켓 + ANALYSIS 산출물(변경 대상 파일/심볼, intent).

## 절차 (5단계)

### 1. 대상 모듈 분류 (cks 불필요)
변경 대상 파일들의 repo-상대 경로를 `applicable-invariants.yaml`의 `path_to_module`로 매핑(prefix, 최장일치 우선). 예: `consensus/wbft/engine/engine.go` → `consensus`.

### 2. 적용 제약 조회 (cks 불필요 — 정적 인덱스)
다음을 합집합으로 수집(`applicable-invariants.yaml`):
- 대상 모듈(들)의 `invariants` 전부,
- `cross_cutting` 규칙 전부,
- **`tier: L3` 항목은 모듈 무관 항상 포함**(always-on backstop — 검색 실패와 무관, INV의 바닥).
- `kind: doc`는 *참고*이지 제약 아님 → 제외. `inv`/`rule`만 제약.

### 3. 인용 부착 (citation 필수)
각 제약에 다음을 단다:
- **invariant id**(정본, `<module>.<area>.<slug>`) — 1차 인용(category-ontology / `_ssot` 추적 가능).
- **code anchor**(`file:line` 또는 `pkg.Type.Method`) — cks 가용 시 `cks.context.find_symbol`/`impact_analysis`로, 미가용 시 `code-knowledge-graph/policies/stablenet/policy.yaml`의 `governs:` 심볼 또는 grep으로 획득.
- **enforce_ref**(있으면) — `chainbench/catalog/invariant-tests.yaml`의 테스트. **binding 등급(good/partial)도 함께** 적되, partial이면 "이 게이트는 distinguishing claim 미검증"을 명시.
- **🔴 인용 없으면 그 항목은 `INVALID` 표시 → DESIGN 진행 차단**(인용 못 하는 제약은 환각으로 간주).

### 4. 영향면 (impact surface)
cks 가용: `impact_analysis(seed)` + `concurrency_impact`로 변경 심볼의 역의존을 부착(seed는 **fully-qualified name** 사용 — 흔한 짧은 이름은 오해석됨[알려진 seed-resolution 버그]). cks 미가용: grep 호출자.
degraded여도 2~3단계(제약 목록)는 항상 산출(D-007).

### 5. `constraints.md` 산출 (산출물 강제)
아래 형식으로 쓴다. 이 파일 없이 DESIGN 단계로 못 넘어간다.

```markdown
# constraints — <ticket-id>
대상 모듈: <modules>  · cks: <available|degraded>

## 적용 불변식·규칙 (제약 우선)
| id | lens | kind | tier | code anchor | enforce | citation? |
|----|------|------|------|-------------|---------|-----------|
| consensus.fee.base_fee_redistribution | fee-policy | inv | L3 | engine.go:972 distributeBaseFee | chainbench:c-03 (⚠partial: not-burned 미검증) | ✅ |
| ... | | | | | | |

## 영향면 (impact surface)
- <symbol> 역의존: <callers/concurrency 또는 grep 결과>

## 설계가 반드시 만족해야 할 것 (게이트)
- [ ] 각 L3 불변식 위반 없음을 *논증*.
- [ ] INVALID 항목 0개(전부 인용됨).
- [ ] partial-binding 불변식은 distinguishing claim을 설계가 별도 보증(테스트 게이트만 믿지 말 것).
```

## 출력 계약
- 모든 제약 항목은 **인용되거나 INVALID**. INVALID가 1개라도 있으면 **반려**(DESIGN 차단).
- L3 always-on 불변식은 검색·cks 상태와 무관하게 **항상** 목록에 있어야 함.
- 산출물은 셀/티켓 워크스페이스의 `constraints.md`.

## 한계 (정직)
- 인덱스는 36-entry 재매핑(category-ontology) 기반 — 빈 모듈(state/crypto/p2p 등)은 제약이 적다(지식 갭, D-024). 그 모듈 변경 시 L3 backstop만 적용됨을 경고로 표기.
- enforce_ref의 partial-binding(3/5)은 테스트 통과가 곧 안전 보증이 아님 — 카탈로그 `missing` 참조.
