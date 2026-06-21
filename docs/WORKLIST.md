# WORKLIST — 통합 작업 리스트 (SSoT)

> 작성: 2026-06-22. 목적: `docs/` 하위 16개 문서에 흩어진 작업 항목을 **5개 스트림**으로 통합한 단일 기준점.
> 상세는 각 원본 문서를 링크. 진행 마커: ✅완료 / ◐부분 / 🟡진입가능·대기 / 🔴미착수 / ☐예정.
>
> 짝 문서: [`remaining-work-detail.md`](./remaining-work-detail.md)(스트림1 상세) ·
> [`cks-ckg-ckv-hardening-backlog-2026-06-19.md`](./cks-ckg-ckv-hardening-backlog-2026-06-19.md)(스트림2) ·
> [`graph-reasoning-gap-and-fix-plan-2026-06-19.md`](./graph-reasoning-gap-and-fix-plan-2026-06-19.md)(스트림3) ·
> [`rag-context-efficiency-proposals-2026-06-19.md`](./rag-context-efficiency-proposals-2026-06-19.md)(스트림4) ·
> [`harness-improvement-proposals-2026-06-17.md`](./harness-improvement-proposals-2026-06-17.md)(스트림5).
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
| F-core 전체 A/B/C bench | thesis 종착점 | 🔴 미실행 |
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
| P0 진단 intent→agentic routing | δpack→γdirect | 🔴 미착수 (※analyzer가 get_for_task+find_callers 직접 routing으로 성공 → P0 가설 부분 입증) |
| P1/P1.5 ckg path 툴·depth-cap 경고 | | 🔴 미착수 (저비용·재인덱싱 불필요) |
| P2 motif 쿼리 3종 | lock 미해제 등 | 🔴 미착수 |
| P3 suffix-match resolver 수정 | ~23% recall | 🔴 미착수 (위험·격리) |
| P4/P5 pack synthesis·data-flow | | ⏸ defer (고비용) |

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

---

## 권장 다음 순서

1. **스트림1 (d) F-core full pipeline 라이브 1셀** — analyzer 검증이 끝났으므로 동일 PR-77 티켓으로 full pipeline(A모드) 또는 A/B/C 비교 진입. **승인+autopilot+오염정리** 전제.
2. 병렬 가능: **스트림4 (1~4)** — coding-agent 단독·차단 없음, 비용 즉시 절감.
3. **스트림2 §3.1/§3.2** — 정합성(정확도 지표 보존). 단 analyzer가 현 인덱스로 PRIMARY 도달했으므로 (d) 실행을 막지 않음.
4. **스트림3 P0** — (d) 라이브 베이스라인 확보 후 재측정과 묶어 "실행→P0→재측정".

**한 줄**: MCP 재연결 + analyzer 검증 완료 → 다음은 **full pipeline 라이브(d)로 thesis 측정 진입**.
