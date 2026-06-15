# perspective-report — STABLE-0001  (EXAMPLE / demonstration)
활성 렌즈: fee-policy · consensus-safety · ledger-conservation · byzantine-fairness · concurrency-discipline  (전체 6 중 5; cross-cutting는 적용 불변식 없음 → 비활성)
> 입력: constraints.md(consensus 모듈) + 평가 대상 설계 변형
> 심문 대상 = 두 변형: (W) gov_validator 미초기화 시 Finalize가 `return nil`(silent skip) · (R) typed sentinel error로 fail-fast

## fee-policy 렌즈  🔴 핵심 (gate-blind 타깃)
- **[VIOLATION] consensus.fee.base_fee_redistribution** — 변형 (W) `return nil`:
  `Finalize → processFinalize`는 `distributeBaseFee`(engine.go:972)를 거쳐 baseFee를 validator에 재분배한다. 미초기화 시 *그 전에* `return nil`로 빠지면 **이미 징수된 baseFee가 재분배되지 않음 = 불변식 위반**(소각/방치). ← `c-03`(chainbench 게이트)은 +2% 공식만 보므로 **이 위반을 통과시킨다**. 이 렌즈가 잡는다.
  - 인용: `consensus.fee.base_fee_redistribution` / engine.go:972 distributeBaseFee / 카탈로그 `missing`(not-burned 미검증).
- **[OK] consensus.fee.base_fee_redistribution** — 변형 (R) sentinel error:
  Finalize가 fail-fast로 에러 전파 → 블록이 *거부*되므로 재분배 누락 상태가 commit되지 않음. 불변식 보존.

## consensus-safety 렌즈
- **[RISK→OK] consensus.finality.instant_finality**: 변형 (W)는 잘못 finalize된(재분배 누락) 블록이 commit되면 inert-reorg로 복구 불가 → finality 위험. 변형 (R)은 fail-fast라 그런 블록이 finalize되지 않음 → OK.
- **[OK] consensus.wbft_core.quorum_calc**: 변경은 quorum 산식(ceil(2N/3))을 건드리지 않음. (게이트 b-08도 good-binding으로 별도 보증.)

## ledger-conservation 렌즈
- **[VIOLATION] (간접) 공급 보존**: 변형 (W)에서 재분배 누락 + `header.Root`(engine.go:967) 재계산 누락 → 다른 노드와 state root 불일치 → 포크/공급 회계 어긋남. 변형 (R) OK.

## byzantine-fairness 렌즈
- **[OK] consensus.validator.equal_power / epoch_transition**: 변경은 validator 권한/epoch 회계를 바꾸지 않음(단, 변형 (W)가 epoch 블록에서 `writeEpoch` 스킵 시 diligence 비대칭 RISK — fail-fast면 무관).

## concurrency-discipline 렌즈
- **[OK] core_lock_discipline**: 순수 nil-guard 추가, 새 goroutine/lock 없음. -race 영향 없음.

## 종합
- blocking VIOLATION: **변형 (W) = 2** (fee-policy, ledger-conservation) → **REJECT**(revise 필요).
- 변형 (R) = 0 → **PASS**(freeze 허용).
- **gate-blind 커버**: `consensus.fee.base_fee_redistribution`의 distinguishing claim(재분배-not-burned)을 chainbench 게이트는 통과시키지만 **fee-policy 렌즈가 변형 (W)를 VIOLATION으로 차단** — #5에서 실증한 사각을 정확히 메움.

---
> **실증 결론:** STABLE-0001의 오답(`return nil`)을 chainbench 불변식 게이트(#4)는 *통과*시키지만,
> perspective-reviewer의 fee-policy/ledger-conservation 렌즈가 **VIOLATION으로 차단**한다.
> = bench C셀이 invariant skill로 잡았던 catch가, 이제 *설계 단계의 명시적 관문*이 됨.
> 기계 게이트(bound 위반) + 렌즈(distinguishing claim) = 상보, 둘 다 통과해야 freeze.
