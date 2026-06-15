# constraints — STABLE-0001  (EXAMPLE / demonstration output)
대상 모듈: consensus  · cks: degraded(서브에이전트 미연결) → 정적 인덱스 + grep
> 변경 대상: `consensus/wbft/engine/engine.go` (Finalize→processFinalize 경로, gov_validator nil-deref)
> path `consensus/wbft/...` → module=`consensus` (path_to_module 최장일치)

## 적용 불변식·규칙 (제약 우선)
모듈=consensus 항목 + 모든 tier:L3 always-on + cross-cutting 규칙.

| id | lens | kind | tier | code anchor | enforce | citation? |
|----|------|------|------|-------------|---------|-----------|
| **consensus.fee.base_fee_redistribution** | fee-policy | inv | **L3** | `engine.go:972 distributeBaseFee` | chainbench:`c-03-basefee-increase` (⚠partial: not-burned 미검증) | ✅ |
| consensus.finality.instant_finality | consensus-safety | inv | L3 | `engine.go:929 processFinalize` (commit seal) | chainbench:`basic/wbft-consensus` (⚠partial: no-reorg 미단언) | ✅ |
| consensus.wbft_core.quorum_calc | consensus-safety | inv | L2 | `validator/default.go:226 QuorumSize=ceil(2N/3)` | chainbench:`b-08-quorum-deficient` (good) | ✅ |
| consensus.validator.equal_power | byzantine-fairness | inv | L3 | `validator/default.go` validator set | chainbench:`b-07` (⚠partial) | ✅ |
| consensus.validator.epoch_transition | byzantine-fairness | inv | L2 | `engine.go:875 buildEpochInfo` | chainbench:`b-03-epoch-transition` | ✅ |
| consensus.concurrency.core_lock_discipline | concurrency-discipline | inv | L3 | `consensus/wbft/core/*` lock 순서 | — | ✅ |
| consensus.wbft_core.round_change | byzantine-fairness | inv | L2 | `core/justification.go` | chainbench:`b-09-round-change` | ✅ |
| consensus.theory.3f1_intersection | consensus-safety | inv | L3 | (이론 backstop) | — | ✅ |
| systemcontracts.native_coin.wkrc_not_eth | ledger-conservation | inv | L3 | `systemcontracts/coin_adapter.go` | — | ✅ (L3 always-on, 모듈 무관) |
| rules.cherry_pick_principle | cross-cutting | rule | L2 | `_ssot/rules` | — | ✅ |

INVALID 항목: **0** (전부 인용됨) → DESIGN 진행 허용.

## 영향면 (impact surface) — degraded(grep)
- `processFinalize` 호출 경로: `Finalize`(engine.go:919) → `processFinalize`(929) → `verifyGasTip`(963)→`getGasTip`(639), epoch 시 `buildEpochInfo`(648)→`:875`. nil-deref 후보: engine.go:639/875.
- 외부 호출자: `core/state_processor.go:102`, `consensus/wbft/backend/engine.go:174`.

## 설계가 반드시 만족해야 할 것 (게이트)
- [ ] **`consensus.fee.base_fee_redistribution` 위반 없음**: Finalize가 `distributeBaseFee`(engine.go:972) *전에* `return nil`로 빠지면 baseFee 재분배 누락 → 위반. **→ graceful 처리는 typed sentinel error여야 하고, redistribution 단계를 건너뛰는 silent skip은 금지.**
- [ ] `consensus.finality.instant_finality` 위반 없음: 잘못 finalize된 빈 블록은 inert-reorg로 복구 불가 → fail-fast(error 전파)로 처리.
- [ ] INVALID 항목 0개(충족).
- [ ] partial-binding 불변식(base_fee_redistribution / instant_finality / equal_power)은 **distinguishing claim을 설계가 별도 보증** — chainbench 게이트 통과만 믿지 말 것(카탈로그 `missing` 참조).

---
> **실증 결론:** consensus 모듈 변경에 대해 `consensus.fee.base_fee_redistribution`(INV-7)이
> *결정적으로* 1행에 표면화됨 + 게이트 항목으로 "silent `return nil` 금지"를 명시.
> = bench C셀이 skill backstop으로 잡았던 catch를, 이제 **모든 모드가 보는 인용된 산출물**로 고정.
> (B=code-only가 추론에 그쳤던 바로 그 제약.)
