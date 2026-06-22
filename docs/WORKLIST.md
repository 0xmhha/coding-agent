# WORKLIST — 통합 작업 리스트 (SSoT)

> 작성: 2026-06-22. 목적: `docs/` 하위 문서에 흩어진 작업 항목을 **6개 스트림**으로 통합한 단일 기준점.
> 상세는 각 원본 문서를 링크. 진행 마커: ✅완료 / ◐부분 / 🟡진입가능·대기 / 🔴미착수 / ☐예정.
>
> 짝 문서: [`remaining-work-detail.md`](./remaining-work-detail.md)(스트림1 상세) ·
> [`cks-ckg-ckv-hardening-backlog-2026-06-19.md`](./cks-ckg-ckv-hardening-backlog-2026-06-19.md)(스트림2) ·
> [`graph-reasoning-gap-and-fix-plan-2026-06-19.md`](./graph-reasoning-gap-and-fix-plan-2026-06-19.md)(스트림3) ·
> [`rag-context-efficiency-proposals-2026-06-19.md`](./rag-context-efficiency-proposals-2026-06-19.md)(스트림4) ·
> [`harness-improvement-proposals-2026-06-17.md`](./harness-improvement-proposals-2026-06-17.md)(스트림5) ·
> [`coding-agent-overlay-improvements-and-eval-2026-06-22.md`](./coding-agent-overlay-improvements-and-eval-2026-06-22.md)(스트림6: 오버레이 개선·도메인팩 확장·평가전략).
>
> ⚠️ `./archive/followup-plan.md`·`./archive/followup-expected-outcomes.md`는 `followup-status-2026-06-15.md`가 정정·대체했으므로
> 아카이브 후보(중복·상충 주의).

---

## 이번 세션 진척 (2026-06-22)

MCP 재연결 트랙 + analyzer 단독 검증을 완주:

1. **소스 동기화 점검** — cks/ckg/ckv/go-stablenet 4 repo 모두 origin 최신(0/0) 확인 (재동기화 불필요).
2. **바이너리 재빌드 검증** — cks-mcp/ckg/ckv 재빌드, schema 1.21·B작업(interfaceMethodSeeds)·canonical_id 임베딩 + `go mod verify` 통과.
3. **DB 검증** — 다른 세션이 생성한 `test-data/{go-stablenet,pr-77}/` 인덱스 양쪽 모두 schema 1.21, ckg↔ckv 동일커밋 쌍, health=ok.
4. **MCP 재연결** — `settings.json` CKS_CONFIG→pr-77 배선, `/mcp` 재연결, 라이브 `cks.ops.health=ok`·`freshness=fresh`(head 0bf2f4d1b).
5. **analyzer 단독 검증 (PR-77, 공정 증상-only 티켓)** — cks 검색만으로 **PRIMARY 근본원인 정확 식별**
   (`anzeon.go:54 SetCurrentBlock`) + **RED 재현 독립 확인**. oracle(`98f05c2a0`) 대조: #1 완전일치, #2(RemotesBelowTip) 부분.
   `get_for_task`+`find_callers` 결정적. 산출물 `test-data/pr-77/{oracle,analyzer-run,ticket.json}`.
6. **오염 정리 완료** — 격리 브랜치/RED 테스트 제거, baseline 무오염 복원, 인덱스 manifest 무변동(C7).

→ **스트림1 항목 11·12의 '검증 대기'가 이번 라이브로 해소**(상세는 remaining-work-detail.md 갱신분).

### 6/19 문서 재검토 결과 (priority 재조정)

`docs/` 6월 19일 클러스터(중단된 작업) 6개를 재검토. 결론:
- **6/19 문서 6개는 대부분 "분석·제안 완료 / 구현 0" 상태**이며, 그중 2개는 작업항목이 아님 —
  `query-core-agent-evaluation`(상위 Claude Code 코어 = **본 repo 범위 밖**), `agent-architecture-and-plugin-guide`(**레퍼런스**).
- **핵심 모순 판정**: 스트림4(rag-efficiency)는 "cks 검색 *품질*은 좋다"고, 스트림3(graph-gap)은 "그래프 미노출로
  진단 *구조적 불가*"라고 정반대 주장. → **6/22 analyzer 공정입력 검증으로 판정**: analyzer가 **13툴
  (get_for_task+find_callers)만으로 PRIMARY 근본원인 도달** → δ<γ를 푼 것은 **새 그래프 프리미티브 노출(P0~P5)이
  아니라 analyzer agentic routing + 시간적-추론/확증편향 보강**. ∴ **graph-gap P0 전제 부분 반증**(스트림3 표 갱신).
- **진짜 "중단된 실행"은 단 하나로 수렴**: F-core (d) full pipeline 라이브 1셀(스트림1). 나머지 6/19 항목은 모두 미착수 제안.

---

## 스트림 1 — coding-agent 파이프라인 / thesis 검증  ★핵심
원본: [`remaining-work-detail.md`](./remaining-work-detail.md) 항목 1–12

| ID | 작업 | 상태 |
|---|---|---|
| MCP 재연결 (재빌드·인덱스·config) | cks/ckg/ckv 운영 반영 | ✅ **완료 (06-22)** |
| 10 analyzer 분리 + reproduce-first 4스테이지 | 구현 | ✅ 구현완료(v0.1.17) — (d) 라이브만 |
| 11 analyzer 시간적-추론 보강 | investigative-probe·확증편향 차단 | ✅ **단독 라이브 검증 통과 (06-22, 공정입력→정답 도달)** |
| 12 ckg iface/동적디스패치 호출 엣지 | find_callers 브릿지 | ✅ 재빌드·재연결 완료(06-22) / ◐ find_callers(GetAnzeonTipCap) 직접검증 잔여 |
| 1 E §4.6 게이트 직접검증 | derived-state | ✅ 완료(06-15) |
| 3 (c) eval-set 확장 | 6모듈/9티켓 | ◐ 대부분완료 (인덱스 drift 잔여) |
| 4 C chainbench 회귀환경 | 상태확인 | 🟢 (d) 비차단 확인 |
| 2 (d) **F-core full pipeline 라이브 1셀** | red→green 완주 | 🟡 **진입가능** (전제충족·analyzer검증됨·승인+autopilot 대기) |
| A/B/C 정의 재설계 (whole-approach) | B/C=coding-agent 배제 단독 solver, C=프로젝트 .claude | ✅ **완료 (06-22)** — 정의문서 + `bench-solver-{codeonly,project-skills}` + SKILL §4.4 분기 + `stable-0005-abc.json` |
| F-core 전체 A/B/C bench | thesis 종착점 | 🟡 **진입가능** (STABLE-0005 매니페스트 준비됨·승인+autopilot 대기) |
| 9 H 가드레일 일반화 | 구현 불변식 확장 | ☐ |

## 스트림 2 — cks/ckg/ckv 하드닝
원본: [`cks-ckg-ckv-hardening-backlog-2026-06-19.md`](./cks-ckg-ckv-hardening-backlog-2026-06-19.md) · [`knowledge-system-analysis-2026-06-17.md`](./knowledge-system-analysis-2026-06-17.md)

| ID | 작업 | 상태 |
|---|---|---|
| §2 세션 재시작/운영 반영 | 머지 PR 6개 반영 | ✅ **완료 (06-22, 재연결로 흡수)** |
| §3.1 ckv identity checksum | 임베딩 공간 교체 감지 | 🔴 미착수 |
| §3.2 ckg silent-incompleteness | 파싱실패 게이트 | 🔴 미착수 |
| §3.3 ckg 성능 6종 | N+1·LIKE·SQLite pragma | 🟠 미착수 |
| §3.4/§3.5 ckg 확장성·ckv 기타 | | 🟠 미착수 |
| analysis Item 9 CKV 15툴 parity gap | flow/invariants 배선 | 🔴 미착수 (※analyzer가 13툴로 PRIMARY 도달 → 우선순위 재평가 여지) |

## 스트림 3 — graph reasoning gap
원본: [`graph-reasoning-gap-and-fix-plan-2026-06-19.md`](./graph-reasoning-gap-and-fix-plan-2026-06-19.md)

| ID | 작업 | 상태 |
|---|---|---|
| P0 진단 intent→agentic routing | δpack→γdirect | 🟢 **사실상 흡수 — 신규구현 불필요** (06-22 공정입력 검증: analyzer가 13툴만으로 PRIMARY 도달 → "진단실패=그래프 미노출" 전제 부분 반증) |
| P1.5 ckg depth-cap 절단 경고 | metadata.warnings (additive) | 🟡 **진입가능** (저비용·재인덱싱 불필요·additive) |
| P1 ckg path/reachability 툴 MCP 노출 | CLI-only→MCP | 🔴 미착수 (중비용) |
| P2 motif 쿼리 3종 | lock 미해제 등 | 🔴 미착수 |
| P3 suffix-match resolver 수정 | ~23% recall | 🔴 미착수 (위험·격리) |
| P4/P5 pack synthesis·data-flow | | ⏸ defer (고비용) |

> **6/22 재평가**: graph-gap의 가치는 P0(진단 라우팅)이 아니라 **P1.5(저비용 가시성)·P2/P3(정확성)** 로 이동.
> P0는 analyzer가 이미 흡수했으므로 우선순위 **하향**.

## 스트림 4 — RAG 비용 효율
원본: [`rag-context-efficiency-proposals-2026-06-19.md`](./rag-context-efficiency-proposals-2026-06-19.md) — 1~6 coding-agent 단독·차단 없음

| ID | 작업 | 상태 |
|---|---|---|
| 1 implementer EvidencePack 재사용 | 중복 full-Read 제거 | 🔴 미착수 (High/Low) |
| 2 evidence 캐시 (index-head 키) | | 🔴 미착수 |
| 3 adaptive search depth 라우터 | | 🔴 미착수 |
| 4 search sufficiency 게이트 | | 🔴 미착수 |
| 5/6 evidence digest·prompt-cache prefix | | 🔴 미착수 |

## 스트림 5 — harness 자율성/안전 (hooks)
원본: [`harness-improvement-proposals-2026-06-17.md`](./harness-improvement-proposals-2026-06-17.md) · [`query-core-agent-evaluation-2026-06-19.md`](./query-core-agent-evaluation-2026-06-19.md)

| ID | 작업 | 상태 |
|---|---|---|
| 1 PreToolUse git-guard | force-push/main 차단 | 🔴 미착수 (autonomy=auto 전제) |
| 2 Stop/SubagentStop 훅 | state 게이트 | 🔴 미착수 |
| 3 SessionStart 컨텍스트 주입 | | 🔴 미착수 |
| 4~7 lessons.md·evaluator 병렬화·schema 검증·context:fork | | 🔴 미착수 |
| query.ts 권고 (#1 reducer·#2 budget) | | ⏸ 본 repo 범위 밖 (상위 Claude Code 코어) |

## 스트림 6 — coding-agent 오버레이 개선 / 도메인팩 확장
원본: [`coding-agent-overlay-improvements-and-eval-2026-06-22.md`](./coding-agent-overlay-improvements-and-eval-2026-06-22.md)

| ID | 작업 | 평가 오라클 | 상태 |
|---|---|---|---|
| P0 계약 기계검증 | plan/design 구조화 스키마 + implementer write-site 표 대조 + evaluator §4.6 완전성 | mutant 코퍼스 누락-행 탐지율 / false-GREEN율 (expert reference_fix) | ✅ **스펙 패치 + 결정론 평가 완료 (06-22)** — planner §4.5/§5.2b·implementer §2.1/§4.2b·evaluator §4.6c. 하네스 `bench/p0-mutants/` (score.py·rules.py·mutate.py·contracts.py·render.py·tests 10/10). **측정: hard mutant 탐지 before 33%→after 100% (+66.7pp), clean 오탐 0**. 잔여: `contract_underdeclare`(planner §5.2b 작성규율, P0 범위밖·정직표기) / 에이전트-인-더-루프 충실도 레이어(render→실에이전트 디스패치) 미실행 |
| P2 cks 결함강건 | 재시도·백오프 + degraded/blocked 임계 명문화 ("조용한 best-effort" 금지) | PR-77 fault-injection (0/10/30/50%) → 조용한 오답 **0** | ✅ **스펙 패치 + 결정론 평가 완료 (06-22)** — analyzer §3.0b(retry+PRIMARY/COMPLETENESS/ENHANCEMENT tier+degraded 전파)·planner §3.0 미러·evaluator §4.0. 하네스 `bench/p2-cks-fault/` (policy·scenarios·score·tests 7/7). **측정: silent-incomplete before 6→after 0, 과잉차단 0, retry 회복 3**. 잔여: 라이브 fault-injection(flaky 프록시+PR-77 오라클) 충실도 레이어 미실행 |
| P5 정리 범위 한정 | evaluator §7.6 pkill→spawn한 PID만 | 동명 더미 프로세스 생존 이진 테스트 | ✅ **완료 (06-22)** — evaluator §7.3 pre-start PID 스냅샷 + §7.6 scoped cleanup(스냅샷 차집합 ∖ $$/$PPID, self-match 버그도 제거). 하네스 `bench/p5-cleanup-scope/` (cleanup_scoped.sh + verify.sh). **실프로세스 이진 테스트 PASS: foreign 생존·ours 종료, naive pkill는 foreign 죽임(버그 실증)** |
| P3 모델 핀 중앙화/갱신 | 6파일 산재 핀 → 단일 설정원, 4-7→4-8 | 동일 픽스처 3-way bench A/B (정확성 무회귀·비용) | ✅ **완료 (06-22)** — 4-8 갱신(`304afba`) + 중앙화: `bench/model-pins/models.json` 단일 소스(tier→id, agent→tier), capture.py가 런타임에 그걸 읽음(이중소스 제거), `check.py`가 frontmatter·capture·prices 정합성 검증(+`--apply` 1-커맨드 갱신). **발견(claude-code-guide 확인): frontmatter `model:`은 런타임 간접참조 불가, `CLAUDE_CODE_SUBAGENT_MODEL`은 tier 평탄화 → 진짜 런타임 중앙화는 불가, 툴링레벨 단일소스+드리프트 게이트가 최선.** 테스트 7/7 + bench 14/14 무회귀 |
| P1 도메인팩 계약 | `domain-pack.json`+`project_id`, `stablenet-*` 정적 호명 → 활성 팩 해석, chainbench→`mcp:` 스테이지 일반화 | ① 코어 grep-clean(도메인 용어 0) ② go-stablenet 무회귀(fcore-baseline) ③ Phase1 라이브 신뢰성 | ◐ **Phase 1 완료 (구조 이동, 06-22)** — 설계 ACCEPTED(`domain-pack-contract-adr-2026-06-22.md`). `plugin/domains/go-stablenet/{domain-pack.json,invariants.md,context.md}` 신설(콘텐츠 단일소스 이동, 불변식 11·경로맵 보존), `stablenet-*` 스킬→thin pointer, 제너릭 `domain-pack` 로더 스킬 신설(미배선). 게이트 `bench/domain-pack/check.py`(5/5)+overlay-gates 편입. **에이전트 frontmatter 무변경=동작 보존**(라이브 무회귀는 미실행). **Phase 2a 브랜치 작업 (`p1-phase2-domain-pack-wire`, 미머지)**: analyzer/planner/evaluator/bench-analyzer-skills frontmatter+본문을 `domain-pack` 로더로 배선, dead 포인터 스킬 삭제, dangling 참조(root-cause-lifecycle·domain-pack) 수정, check.py를 wiring 검증으로 갱신. frontmatter stablenet-* 의존 0·overlay-gates ALL PASS. **검증 3/4 통과**(`phase2a-verification.md`): ①콘텐츠 byte-identical ②로더 해석 결정론 ③**실제 에이전트가 로더만으로 resolve·분류·불변식 적용 PASS(§3.1 실측)**. **🔴 ④풀파이프라인 무회귀(fcore-baseline)만 라이브 세션 필요 → 통과 전 머지 금지.** 잔여 Phase 2b(evaluator `go_stablenet_root`·verification_stages 일반화)·3(검증)은 게이트 유지 🔴 |
| P4 문서 드리프트 | HANDOFF-simulation-verification supersede (reproduce-first/§4.7로 충족분 반영) | doc-truth 대조표: 활성문서 ↔ 코드 모순 0건 | ✅ **완료 (06-22)** — `plugin/docs/HANDOFF-simulation-verification.md` 상단에 STATUS+doc-truth 대조표(7행) prepend, §8 NEXT supersede. 제안 (1)~(5)는 reproduce-first 트랙으로 충족 확인, **활성 잔여 1건만 분리**: 신규 `simulation-harness` 스킬(L1/L2/L3 라우팅 + L2 in-process 시뮬 + ChainBench L2 down-push) → 아래 잔여항목 추적 |

> **시퀀싱 핵심**: P0·P1·P3은 파이프라인 동작을 *바꾼다* → 스트림1 thesis bench(F-core)의 측정을
> 교란한다. **F-core 베이스라인을 먼저 캡처**한 뒤 착수하거나, 각 항목을 *자체 before/after A/B*로
> 측정해야 한다(Part C 변수격리). P2·P4·P5는 비교란이라 아무 때나 가능.
>
> **📌 베이스라인 핀 (06-22)**: git tag **`fcore-baseline` = `b33931f`** (overlay P0/P2/P4/P5 적용,
> pre-P1/P3). A/B/C 라이브 런(다른 세션)은 이 커밋에서 돌리고, P1/P3 교란 작업은 이것을 *before*로
> 비교한다. 순수 pre-overlay 기준점이 필요하면 `def2af0~1`. (캡처=참조 커밋 핀일 뿐, 벤치 *실행*은
> 별개 — 스트림1 F-core 행 참조.)

**P4 분리 잔여 (신규 추적 항목):** `simulation-harness` 스킬 — 재현 테스트의 시뮬레이션 레벨
라우팅(L1 단위 / L2 in-process 체인·합의 / L3 ChainBench) + ChainBench(L3, 20분)를 L2로 down-push.
red→green 골격은 reproduce-first로 이미 존재하므로 *레벨 카탈로그/라우팅*만 추가하면 됨. 🔴 미착수
(원 제안: `plugin/docs/HANDOFF-simulation-verification.md` §6 신규 스킬·§9 결정성 가이드).

**✅ 오버레이 회귀 게이트 통합 (06-22):** `bash bench/overlay-gates.sh` — P0/P2/P3/P5 하네스 +
bench 단위테스트 9개를 한 커맨드로 묶어 스펙 회귀를 즉시 잡는다(음성 테스트로 드리프트 주입→FAIL 확인).
pre-commit/CI 후보. P0~P5의 "진짜 개선" 보증을 영구 고정하는 capstone.

---

## 권장 다음 순서 (2026-06-22 6/19 재검토 반영 — 조정본)

| 순위 | 작업 | 근거 |
|---|---|---|
| **1** | **스트림1 (d) F-core full pipeline 라이브 1셀** | 유일한 진짜 미완 *실행*·thesis 종착점. 전제(analyzer검증·MCP재연결·bench코드) 충족 → **승인+autopilot+오염정리만** 남음 |
| **2 (병렬)** | **스트림4 3.1 + 2.2 + 4.1** (저노력·고효과) | coding-agent 단독·차단 없음·즉시 비용절감. (3.1=implementer EvidencePack 재사용이 최우선) |
| **3** | **스트림2 §3.1 ckv identity + §3.2 ckg silent-incompleteness** | (d) 측정의 정확도 지표 신뢰성 보존. (d) 실행을 막지는 않음 |
| **4 (하향)** | **스트림3 P1.5 → P2/P3** (P0 제외) | P0는 analyzer가 흡수(반증). 살아남는 건 저비용 가시성(P1.5)·정확성(P2/P3) |
| **보류** | parity gap · query-eval · 스트림5 hooks | parity는 analyzer 13툴 PRIMARY 도달로 근거 약화 / query-eval 범위밖 / hooks는 autonomy=auto 전제 |

### 스트림6 오버레이 트랙 통합 순서 (thesis 트랙과의 관계)

오버레이 개선(P0~P5)은 위 thesis/cks 트랙과 **별개 축**이지만, **교란 의존성**으로 끼워 넣어야 한다:

| 단계 | 작업 | 근거·의존 |
|---|---|---|
| **A. 즉시·병렬** | 스트림6 **P2·P5** + 스트림6 **P4** | 파이프라인 비교란 → thesis 측정과 무관하게 아무 때나. P2는 PR-77 오라클로 단독 평가 |
| **B. 베이스라인 락 이후** | 스트림6 **P0** | 계약 기계검증은 파이프라인 동작을 바꿈 → **스트림1 F-core 베이스라인 캡처 후** 착수(또는 자체 A/B) |
| **C. 갱신 게이트** | 스트림6 **P3** | 모델 핀 갱신은 동일 픽스처 bench A/B로 무회귀 게이트 통과 시에만 |
| **D. 대형 확장** | 스트림6 **P1** | 도메인팩 계약 = 최대 작업. 반드시 *무리팩터 이동→무회귀 락→일반화* 순. P0 구조화 산출물·P2 degraded 전파와 맞물림 |

> **두 트랙 합본 1줄 우선순위**: ①스트림1 F-core 라이브(thesis 종착·overlay 교란분의 베이스라인) →
> ②스트림6 P2·P5 + 스트림4 저비용(병렬·비교란) → ③스트림6 P0 + 스트림2 §3.1/§3.2(정확도 신뢰성) →
> ④스트림6 P3(게이트) → ⑤스트림6 P1(도메인팩 대형) → ⑥스트림6 P4 + 잔여 문서정리.

**한 줄**: 6/19 문서는 "분석 끝·구현 0", graph-gap P0 전제는 6/22 검증으로 흔들림 → 진짜 중단된 건 **F-core (d) 하나뿐 = 1순위**. 그 위에 **오버레이 트랙(스트림6)**은 *비교란 P2·P5·P4 즉시 / 교란 P0·P3·P1은 F-core 베이스라인 캡처 후* 끼워 넣는다.
