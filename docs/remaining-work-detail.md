# 남은 작업 상세 (Remaining Work — 무엇/왜/문제/기대결과)

> 작성: 2026-06-15. 짝 문서: [`followup-status-2026-06-15.md`](./followup-status-2026-06-15.md)(실측 대조·진행),
> [`followup-plan.md`](./followup-plan.md)·[`followup-expected-outcomes.md`](./followup-expected-outcomes.md)(원 계획).
> 각 항목은 **무엇 / 왜 / 해결하는 문제 / 기대결과**로 정리. 우선순위순.

## 추천 실행 순서

**~~E~~ ✅ → ~~(c) eval-set~~ ◐ → ~~C 상태확인~~ ✅(블로커 아님) → (d) 1셀 완주 → 전체 실행 → cks측 B/D.**
> 2026-06-15: E 완료(§4.6 실증) + (c) 6개 모듈 태스크(phase3) + C 상태확인 완료(벤치 oracle은 안 막힘).
> 다음 권장 = **(d) clean 태스크(STABLE-0004/0005/0007) 1셀 dry→완주** — autopilot 세션 + 승인 필요.

이유: F-core 하네스 코드는 랜딩됐으나 아직 end-to-end로 안 돌았고, A/B/C 세 모드가 **공유하는 단 하나의
측정 계기 = evaluator**다. 이 계기를 믿을 수 있게 만든 뒤(E), 좋은 입력(c·C)을 갖춰 **본 실행(d)으로
thesis를 증명**하는 흐름. "비용 대비 의사결정 가치"로 E가 1순위(외부 의존 0·go-stablenet 변경 0·저비용).

진행 상태 범례: ✅완료 / ◐부분 / ⏳대기 / ☐미착수

---

## ✅ 1. E — evaluator §4.6 derived-state 게이트 직접 검증  *(완료 2026-06-15)*
- **무엇**: "maintained 집계(파생상태)를 추가했는데 일관성 invariant 테스트가 없는" 가상 diff를 evaluator에
  태워, §4.6 게이트가 **FAIL→bug cycle**을 발화하는지 실증.
- **왜**: §4.6은 출시됐으나 RETEST에서 planner가 파생상태를 회피(lazy-on-read)해 **한 번도 발화된 적 없음**(미실증).
- **해결하는 문제**: `truncatePending`류 파생상태 부작용에 대한 **2차 안전망의 신뢰성**, 그리고 F 본 실행의
  correctness 신호가 믿을 만한지에 대한 사전 보증.
- **결과**: ✅ **실증 완료.** negative.diff(테스트 없음)→**FAIL**, positive.diff(invariant+적대경로 테스트)→
  **PASS**, 둘 다 기대치 100% 일치. 과탐/미탐 없음. 산출물: `bench/fixtures/eval-gate/`(diff 쌍 +
  `expected.json` + `result-2026-06-15.md`). 검증: 실제 evaluator 에이전트 2회 디스패치(go-stablenet 변경 0).
- **후속(선택)**: 캐시/인덱스/카운터 형태·design §5.2b 경로 선언 케이스로 fixture 확장 가능(현재 add/sub aggregate만 커버).

## 🔴 2. (d) F-core 본 실행 — 1셀 완주 → 전체  *(핵심 마일스톤)*
- **무엇**: bug-cycle 루프가 들어간 하네스로 실제 A/B/C 셀을 ANALYSIS→…→EVALUATION_PASS까지 완주(현재 0건),
  compare.py로 총비용·cycle·side-effect 비교표 산출.
- **왜**: 프로젝트의 **존재 이유(thesis: "cks가 grep보다 정확·저렴한가")를 숫자로 증명/반증** — 사실상 종착점.
- **해결하는 문제**: "동작하지만 미증명" 상태(과거 N=1 측정은 FakeEmbedder 등으로 confounded).
- **기대결과**: A/B/C의 **bug-cycle 수·Σ토큰·회귀실패·최종정확성** 정량 비교 → go/no-go 데이터(반증도 가치).
- **전제/주의**: autopilot 세션 + **사용자 승인** + go-stablenet 변경 + **데이터셋 오염 정리 필수**. (c)·C 의존.

## 🟡 3. (c) eval-set 확장 — 모듈 다양화 + base_commit/oracle  *(◐대부분 완료 2026-06-15)*
- **무엇**: consensus+txpool → systemcontracts·state·miner·params 등으로 확장. 실제 PR의 **부모 커밋**을
  `base_commit`(버그 실재), 실제 fix를 oracle로.
- **왜**: 단일/소수 모듈은 **케이스 편향**. thesis 일반화엔 N≥3 + 다모듈 + **grep 경쟁 대조군** 필요.
- **해결하는 문제**: 단일 케이스 결론 위험. base_commit이 틀리면 버그가 이미 고쳐진 코드라 **trivially pass**(거짓 신호).
- **결과**: ◐ **6개 모듈 태스크 추가 완료** — `STABLE-0004`(systemcontracts/#83), `0005`(miner-Anzeon/#77),
  `0007`(state-genesis/#73), `0009`(chainconfig/#58), `0006`(params-hardfork/#68), `0008`(fee-policy/#14).
  매니페스트 `stablenet-abc-phase3.json`, 각 base_commit go-stablenet에 실재 검증. 검색 스펙트럼:
  그래프-heavy(consensus)·파생상태(txpool)·**교차언어(systemcontracts)**·**교차모듈(Anzeon)**·**grep대조군(params)**.
- **티켓 설계 원칙(2026-06-15 적용)**: 모든 티켓을 **증상-수준**으로 작성 — 증상·재현·영향영역만 주고
  **수정 메커니즘(함수명·코드 변경)은 누설 금지**. 누설하면 (i) 검색 품질이 무의미해져 cks 변별력↓,
  (ii) 전문가-유사도 테스트(PR-77식) 무효. 9개 티켓 전수 스캔으로 누설 0 확인. feature(0006/0008)는
  목표-수준(설계 자유도 큼 → 유사도는 약신호). 전문가 정답은 `oracle.reference_fix`로 평가 시점에만 참조.
- **남은 것**: ⚠️ **인덱스 drift** — cks 인덱스는 dev(c051d50b)에 빌드됨. base가 dev에서 먼 태스크
  (#68/#58/**#14**)는 retrieval drift↑ → 정밀 측정 시 base_commit에 인덱스 재빌드 고려. clean 태스크
  (#83/#77/#73)를 1차 실행 권장. oracle(chainbench_test)은 (d) 실행 시 live 검증.

## 🟢 4. C — chainbench 회귀 환경 정비  *(상태 확인 완료 2026-06-15 → 거의 안 막힘)*
- **무엇**: chainbench 회귀 환경이 (d) 본 실행의 정확성 축을 막는지 확인.
- **06-12 문서 대비 정정 (실측)**:
  - ✅ **`WKRC.yaml` 부재는 오독** — WKRC는 *코인 심볼*(스테이블코인)이지 프로파일명이 아님. v2 회귀
    환경은 이제 **`profiles/regression.yaml`로 존재**(4 BP+1 EN, AnzeonBlock=0/BohoBlock=0, 103 TC).
  - ✅ **sender 펀딩 해소** — regression.yaml alloc이 TEST_ACC_A~E(sender/recipient/fee-payer/authorize/
    blacklist) 모두 펀딩. 회귀 스위트도 `a-ethereum…h-hardfork/z-layer2` 9범주로 재구성("a2-*"는 stale 명칭).
  - ✅ **벤치 oracle 비차단(핵심)** — 배선한 oracle `basic/consensus`·`basic/tx-send`는 둘 다
    `get_running_node_ids`를 **0회** 사용 → 그 버그와 무관하게 실행 가능. **F 정확성 축은 막혀 있지 않다.**
- **남은 것(우리가 안 고침, chainbench 소유)**:
  - ❌ `get_running_node_ids` 여전히 repo 전체 미정의 → `basic/txpool-propagation`·`basic/wbft-consensus`·
    `fault/{network-partition,p2p-topology}` 4개만 exit 127. **벤치 oracle 아님 → 영향 없음.**
  - ⚠️ regression.yaml `binary_path`가 타 머신 경로(`wm-it-22-00661`) → 이 머신엔 gstable 빌드 존재
    (`go-stablenet/build/bin/gstable` ✓). `node.start binary_path` 런타임 override로 우회 가능(커밋 `3150719`).
- **결론**: C는 (d) clean 태스크 실행의 **블로커가 아니다.** 위 4개 스크립트가 필요한 회귀까지 덮으려면
  그때 chainbench 소유 측에 `get_running_node_ids` 정의를 요청. 그 전까지 해당 항목만 "부분 검증" 표기.

## 🟡 5. B-verify — intent 분류 정상동작 + demoteTests 결합 확인  *(cks 측, 병행, ☐)*
- **무엇**: 재작성된 임베딩 intent 분류기가 SN/KO 쿼리에서 정상 분류(비-empty, above-threshold)하는지,
  `demoteTests := intent != TestAdd` 결합 부작용이 있는지 측정.
- **왜**: intent가 틀리면 composer의 symbol-kind/relation 필터가 광역모드로 죽고, test-add 태스크에서 테스트 오강등.
- **해결하는 문제**: 잔여 recall 갭의 유력 근본원인 + 검색 노이즈.
- **기대결과**: intent 분류율↑ → 필터 활성 → 노이즈↓·랭킹↑ (file recall 0.8→0.9+ 목표).

## 🟢 6. D-2 — KO recall 회귀(0.71→0.57)  *(cks 측, 신규 발견, ☐)*
- **무엇**: production-over-test 강등 + glossary 확장 영향을 **분리 측정**(demoteTests on/off),
  ko01-quorum·ko04-commit MISS 원인 규명.
- **왜**: SN을 +0.10 올린 변경이 **한국어를 회귀**시킴 — 의도된 트레이드오프인지 버그인지 미확인.
- **해결하는 문제**: 한국어 시나리오 검색 품질 저하.
- **기대결과**: 회귀 원인 격리 → KO recall 회복, on/off 정책 결정.

## 🟢 7. txpool fix 브랜치 처리  *(정리, ☐)*
- **무엇**: 로컬·미푸시 브랜치 `920ec4320`(3커밋) 방향 결정 — 내 수정(maintained map+Cap 훅) vs
  0.1.10 planner의 lazy-on-read 설계 수렴점.
- **왜**: 실제 발견한 고-가치 버그 수정이 **dev 미반영 방치** 중. (플랜의 G=PR 조율은 명시 제외 → 방향 결정까지만.)
- **해결하는 문제**: FD tx 오탐 거부(DoS 방어가 정상 트래픽을 막는 가용성 버그).
- **기대결과**: 머지/보류 결정 + STABLE-0003 벤치 태스크 oracle 정합.

## 🟢 8. D — SN 잔여 miss(sn06 gas price, sn07 blacklist)  *(수확체감, ☐)*
- **무엇**: 임베딩/청킹(contextual retrieval) 개선으로 file-level 진짜 miss 회복.
- **왜/문제**: intent로 흡수 안 되는 임베딩 한계 영역.
- **기대결과**: file recall 0.9+ 접근 — 단 **큰 비용 대비 작은 이득** 가능(낮은 우선순위).

## 🟢 9. H — 가드레일 일반화  *(마무리, ☐)*
- **무엇**: invariant-backstop을 합의 불변식 → **구현 불변식**으로 확장, 메모리/문서 정리.
- **왜/문제**: 현재 backstop이 합의 도메인 한정 → txpool 외 태스크엔 미적용.
- **기대결과**: 신규 derived-state 태스크에서 §5.2b/§4.6 자동 적용 → 전 태스크 유형 부작용 예방.

---

**한 줄 요약**: 계기를 먼저 믿을 수 있게 만들고(**E**), 그 위에서 좋은 입력(**c·C**)을 갖춰 **본 실행(d)으로
thesis를 증명**한다. cks측 검색품질(B/D-2)은 동시 세션과 병행 가능한 곁가지.
