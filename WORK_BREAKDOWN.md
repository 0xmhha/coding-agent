# Coding Agent - Work Breakdown

> Phase 설계 문서 기반 전체 작업 목록.
> 각 작업의 출처(Phase), 유형, buddy 재사용 여부를 명시.

## 작업 유형 범례

| 유형 | 의미 |
|------|------|
| `NEW` | 새로 작성 |
| `ADAPT` | buddy 스킬을 coding-agent에 맞게 적응 |
| `REF` | buddy 스킬의 패턴/구조만 참고 |
| `INFRA` | 인프라/프로젝트 설정 |

---

## Phase 1: Plugin Skeleton + State Machine

### P1-1. 프로젝트 초기 구조 [INFRA]
- plugin.json 매니페스트 작성
- 디렉토리 생성: commands/, agents/, skills/, hooks/, shared/
- .gitignore에 .coding-agent/ 추가

### P1-2. Command: /work [NEW]
- commands/work.md 작성
- JIRA-ID 형식 검증 (/^[A-Z]+-\d+$/)
- 중복 작업 체크 (기존 작업 폴더 탐색)
- 작업 폴더 생성: .coding-agent/tickets/{JIRA-ID}_{YYYYMMDD_HHmmss}/
- state.json 초기화
- Orchestrator Agent 디스패치 진입점

### P1-3. Command: /review [NEW]
- commands/review.md 작성
- PR URL 파싱 + 리뷰 코멘트 수집 (gh API)
- 브랜치명에서 JIRA-ID 추출
- 기존 작업 폴더 탐색 + review-feedback-{N}.md 생성
- ANALYSIS 상태로 재진입

### P1-4. Command: /status [NEW]
- commands/status.md 작성
- 특정 JIRA-ID 또는 전체 활성 작업 조회
- plan_progress step별 진행률 출력
- failure_summary 요약 출력

### P1-5. Command: /merge [NEW]
- commands/merge.md 작성
- PR 전제조건 검증 (approved, checks, mergeable)
- squash merge 실행 + Jira 상태 Complete

### P1-6. Skill: state-machine [NEW]
- skills/state-machine.md 작성
- init_state, get_current_state, transition, update_step_progress, log_failure, get_resume_point
- 전이 조건 검증 로직
- failure_summary 자동 업데이트 (recurring_patterns 포함)
- 참고: buddy의 `status` 스킬 구조

### P1-7. Skill: template-parse [NEW]
- skills/template-parse.md 작성
- Jira 티켓 유형 식별 (Feature/Bug Fix/Code Review/Release)
- 템플릿 필드 구조화 파싱
- 유형별 파이프라인 분기 조건 제공

### P1-8. Skill: stablenet-context [NEW]
- skills/stablenet-context.md 작성
- go-stablenet 도메인 분류 (consensus, core, p2p, rpc, governance, txpool, state ...)
- 모듈별 특성, 동시성 패턴, 주의사항
- 실제 프로젝트 탐색 시 상세화 (경로 전달 후)

### P1-9. Agent 스텁 파일 [INFRA]
- agents/orchestrator.md (스텁)
- agents/planner.md (스텁)
- agents/implementer.md (스텁)
- agents/evaluator.md (스텁)

### P1-10. Hook 골격 [INFRA]
- hooks/hooks.json 작성 (disabled 상태로 이벤트 정의)

---

## Phase 2: Jira Gateway MCP + Sensitive Filter

### P2-1. Jira Gateway MCP 서버 프로젝트 생성 [NEW]
- jira-gateway-mcp/ 디렉토리 구조 생성
- TypeScript + @modelcontextprotocol/sdk
- package.json, tsconfig.json 설정

### P2-2. Jira REST API 클라이언트 [NEW]
- src/upstream/jira-client.ts
- 인증: JIRA_BASE_URL + JIRA_API_TOKEN + JIRA_USER_EMAIL (Basic Auth)
- 티켓 읽기, 코멘트 읽기/쓰기, 상태 변경, 검색

### P2-3. Sensitive Filter 엔진 [NEW]
- src/filter/engine.ts - 메인 필터 엔진
- src/filter/patterns.ts - patterns.json 로더 + regex 매처
- src/filter/entropy.ts - Shannon entropy 계산
- src/filter/redactor.ts - REDACT/BLOCK/WARN 처리
- 참고: buddy의 `audit-security`, `design-secret-management`

### P2-4. patterns.json 정의 [NEW]
- shared/patterns.json 작성
- Critical: PEM key, AWS key, OpenAI key, Anthropic key, GCP service account
- High: DB 접속 문자열, JWT, Bearer token, webhook secret, password
- Medium: 내부 IP, 이메일
- Entropy: 고랜덤 문자열 (threshold 4.5, exclude hex hash/URL)

### P2-5. MCP Tool 등록 [NEW]
- jira_read_ticket (필터 적용)
- jira_read_comments (필터 적용)
- jira_search (필터 적용)
- jira_add_comment (passthrough)
- jira_update_status (passthrough)
- jira_update_assignee (passthrough)

### P2-6. 필터 단위 테스트 [NEW]
- tests/filter.test.ts
- tests/entropy.test.ts
- tests/redactor.test.ts
- REDACT/BLOCK/WARN 각 경로 검증
- fail-safe 동작 확인 (필터 오류 시 차단)

### P2-7. 패턴 커스터마이징 메커니즘 [NEW]
- .coding-agent/custom-patterns.json 지원
- 기본 + 커스텀 merge 로직

---

## Phase 3: CKS MCP - CKV (Vector Search)

### P3-1. CKS MCP 서버 프로젝트 생성 [NEW]
- cks-mcp/ 디렉토리 구조 생성
- Go + modelcontextprotocol/go-sdk
- go.mod 설정

### P3-2. Go AST Code Chunker [NEW]
- internal/ckv/chunker.go
- FuncDecl, GenDecl(type/const/var), File-level 청킹
- 200줄 초과 함수 서브 청크 분할
- _test.go 포함, _gen.go/_mock.go 제외
- CodeChunk 구조체 정의

### P3-3. Embedding 통합 [NEW]
- internal/ckv/embedder.go
- Tier 1 로컬: Ollama + nomic-embed-text (또는 CodeBERT)
- Tier 2 API: Voyage Code 3 (선택적)
- 임베딩 입력 포맷: Package + File + Type + Signature + godoc + code
- 참고: buddy의 `design-embedding-search` PROCEDURE.md

### P3-4. Vector Store (SQLite + sqlite-vss) [NEW]
- internal/ckv/store.go
- chunks 테이블 + chunk_embeddings 가상 테이블
- 메타데이터 인덱스 (package, file, symbol_type)

### P3-5. 검색 파이프라인 [NEW]
- internal/ckv/search.go
- 벡터 검색 → 메타데이터 enrichment → Reranking → Sensitive Filter
- 필터: package, file_pattern, symbol_type, modified_since

### P3-6. Reranker [NEW]
- Cross-encoder (기본) 또는 LLM 기반 (선택)
- 시그니처 부스팅 ×1.5, godoc 부스팅 ×1.3
- 최근 수정 부스팅 ×1.1, 패키지 근접성 ×1.2

### P3-7. Indexing Pipeline [NEW]
- internal/ckv/indexer.go
- Full index: git ls-files → parse → chunk → embed → store
- Incremental index: git diff 기반 변경분만 업데이트
- .coding-agent/index/ 저장

### P3-8. MCP Tool: ckv_search [NEW]
- query, top_k, filters, include_history, rerank 파라미터
- 결과: file, symbol, snippet, score, git_history_summary

### P3-9. MCP Tool: ckv_index [NEW]
- full/incremental 모드
- 인덱싱 통계 반환

### P3-10. Sensitive Filter 내장 [ADAPT]
- internal/filter/engine.go
- Phase 2의 patterns.json 공유 (shared/)
- Go 재구현 (Phase 2는 TypeScript, Phase 3은 Go)

---

## Phase 4: CKS MCP - CKG (Graph Search)

### P4-1. Graph Store (SQLite Adjacency) [NEW]
- internal/ckg/store.go
- graph_nodes, graph_edges, symbol_history, concurrency_context 테이블
- 재귀 CTE 기반 그래프 탐색

### P4-2. AST Relation Extractor [NEW]
- internal/ckg/relations.go
- 7개 관계 유형: calls, implements, uses_type, embeds, reads_field, writes_field, channels
- go/types 패키지로 cross-package 타입 resolve
- golang.org/x/tools/go/packages 활용

### P4-3. Git History Analyzer [NEW]
- internal/ckg/history.go
- git log -L (줄 범위 기반) 심볼별 변경 히스토리
- git log --follow (파일 이름 변경 추적)
- 커밋 유형 분류: signature_change, logic_change, refactor, bugfix, feature
- 참고: buddy의 `summarize-retro` 패턴

### P4-4. Concurrency Analyzer [NEW]
- internal/ckg/concurrency.go
- goroutine 시작점 탐지 (go func, go obj.Method)
- channel send/receive 쌍 매칭
- sync.Mutex/RWMutex 범위 분석
- 공유 자원 식별 + 보호 여부 확인
- race condition 리스크 평가

### P4-5. Traversal Query Engine [NEW]
- internal/ckg/traversal.go
- BFS/DFS + depth 제어
- 관계 유형 필터
- 결과 크기 제한 (max_nodes: 200, max_edges: 500)

### P4-6. MCP Tool: ckg_query [NEW]
- symbols, depth, relation_types, include_history, include_concurrency
- nodes + edges + history + concurrency_impact 반환

### P4-7. MCP Tool: ckg_impact [NEW]
- symbol + change_type → 영향 범위 분석
- direct/indirect callers, interface contracts, test files
- concurrency risk, recommended test scope, risk level

### P4-8. MCP Tool: ckg_index [NEW]
- full/incremental 모드
- CKV와 통합 인덱싱 (AST 1회 파싱)

### P4-9. CKV + CKG 통합 검색 흐름 [NEW]
- CKV(의미) → 핵심 심볼 추출 → CKG(구조) 순차 호출 패턴
- Planner Agent가 사용하는 표준 검색 시퀀스

---

## Phase 5: Agent Pipeline

### P5-1. Orchestrator Agent 구현 [NEW]
- agents/orchestrator.md 완성
- state.json 기반 상태 분기 + 서브 에이전트 디스패치
- COMPLETION: PR 생성 + Jira 업데이트
- BLOCKED: 유저 보고 + 지시 대기
- 참고: buddy의 `router` 디스패치 패턴

### P5-2. Planner Agent - ANALYSIS 구현 [ADAPT]
- agents/planner.md 완성 (ANALYSIS 섹션)
- template-parse → CKV 검색 → Sonnet 검토 → CKG 탐색 → CKG Impact
- analysis.md + related-code.json 생성
- 참고: buddy의 `plan-build`

### P5-3. Planner Agent - PLANNING 구현 [ADAPT]
- agents/planner.md (PLANNING 섹션)
- 작업 분해 → atomic step → 의존성 위상 정렬 → 검증 테스트 정의
- plan.md 생성
- 참고: buddy의 `decompose-track-to-tasks`, `map-task-dependencies`

### P5-4. Planner Agent - DESIGN 구현 [NEW]
- agents/planner.md (DESIGN 섹션)
- 정밀 코드 설계 (수정 전/후 의사 코드, side-effect 체크리스트)
- Self-review 반복 (max 3회)
- design-v{N}.md + design-changelog.md 생성

### P5-5. Implementer Agent 구현 [ADAPT]
- agents/implementer.md 완성
- plan + design 기반 코드 구현
- 분할 커밋 전략 + checkpoint 기록
- 중단 복구: last_checkpoint에서 재개
- 참고: buddy의 `iterate-fix-verify`, `build-with-tdd`

### P5-6. Checkpoint/복구 메커니즘 [ADAPT]
- state.json의 plan_progress.steps + last_checkpoint
- /work 재실행 시 복구 진입점
- 참고: buddy의 `save-context`, `restore-context`

### P5-7. Bug Cycle 재진입 로직 [ADAPT]
- EVALUATION_FAIL → ANALYSIS 재진입
- failure_log + git diff(로컬) + CKS(원본) 종합 분석
- plan-fix-{cycle}.md 생성
- 참고: buddy의 `diagnose-bug`

### P5-8. 작업 유형별 파이프라인 분기 [NEW]
- Code Review: ANALYSIS → PLANNING(리뷰 리포트) → COMPLETION
- Release: ANALYSIS → EVALUATION → COMPLETION(태그 + 릴리즈)

### P5-9. Hook 구현 [NEW]
- hooks/hooks.json 활성화
- hooks/on-agent-complete.js (에이전트 완료 시 state 업데이트)
- hooks/on-commit.js (커밋 시 진행 로깅)

---

## Phase 6: Evaluator + ChainBench

### P6-1. Evaluator Agent 구현 [ADAPT]
- agents/evaluator.md 완성
- 4-stage 파이프라인 순차 실행
- 모든 stage 실행 (중간 중단 없음)
- 참고: buddy의 `verify-quality`, `measure-code-health`

### P6-2. Stage 1: Unit Test [NEW]
- go test 실행 + 결과 파싱 (passed/failed/coverage)
- 변경 패키지 대상 + 전체 테스트
- 커버리지 delta 분석

### P6-3. Stage 2: Lint & Format [ADAPT]
- golangci-lint + gofmt + goimports
- 결과 파싱 + 판정 (error → FAIL, warning → PASS)
- 참고: buddy의 `measure-code-health`

### P6-4. Stage 3: Security Scan [ADAPT]
- go vet + gosec + 코드 패턴 검사
- 하드코딩 시크릿, unsafe, 입력 검증, 에러 무시, 동시성 안전
- 참고: buddy의 `audit-security`, `classify-review-risks`

### P6-5. Stage 4: ChainBench Integration Test [NEW]
- ChainBench MCP 연동
- 빌드 → 네트워크 구성 → 안정화 → 블록 생성 모니터링 → tx 테스트 → 정리
- 타임아웃 + cleanup 보장

### P6-6. test-report.md 생성기 [NEW]
- 4-stage 결과 종합 리포트
- 마크다운 포맷 + stage별 상세

### P6-7. failure_log 자동 기록 [ADAPT]
- 다중 FAIL 처리 (모든 FAIL을 하나의 entry에)
- failure_summary 자동 업데이트
- recurring_patterns 갱신
- 참고: buddy의 `persist-learning-jsonl`

---

## Phase 7: PR + Review Cycle

### P7-1. PR 자동 생성 [ADAPT]
- git push + gh pr create
- PR body: Jira 링크, 변경 요약, 테스트 결과, 영향 분석, Acceptance Criteria
- PR 라벨 자동 설정
- 참고: buddy의 `auto-create-pr`

### P7-2. Jira 연동 (PR 후) [NEW]
- jira_add_comment: PR URL 게시
- jira_update_status: "In Review"

### P7-3. /review 코멘트 파싱 + 구조화 [NEW]
- gh API로 인라인 코멘트 + 리뷰 수집
- 코멘트 분류: bug_fix, security, test_addition, code_quality, architecture, question, nit
- 심각도: critical/high/medium/low
- review-feedback-{N}.md 생성

### P7-4. 리뷰 기반 재작업 사이클 [ADAPT]
- Planner 리뷰 모드: 코멘트 분석 → plan-review-{N}.md
- 구현 → 테스트 → PR 업데이트
- 코멘트별 응답 자동 게시 (gh API)
- 참고: buddy의 `iterate-fix-verify`

### P7-5. /merge 구현 [ADAPT]
- 전제조건 검증 (approved, checks, mergeable)
- gh pr merge --squash --delete-branch
- squash commit body: 개별 커밋 목록 포함
- 참고: buddy의 `finish-development-branch`

### P7-6. Merge 후 처리 [ADAPT]
- Jira 상태 → Complete
- Jira 댓글: merge commit hash
- 로컬 브랜치 정리
- state.json → COMPLETED
- 참고: buddy의 `automate-release-tagging`

### P7-7. PR body 민감정보 스캔 [NEW]
- PR body/commit 메시지에 shared/patterns.json 스캔
- 민감정보 발견 시 제거 후 PR 생성

---

## 공통/인프라 작업

### COMMON-1. shared/patterns.json [NEW]
- 민감정보 패턴 SSoT
- Jira Gateway MCP와 CKS MCP 공유
- 커스텀 패턴 merge 로직

### COMMON-2. .coding-agent/ 폴더 관리 유틸리티 [NEW]
- find_workspace(ticket_id, status_filter)
- find_active_workspaces()
- 폴더 생성/탐색/정리

### COMMON-3. 안전장치 (Safeguard) [ADAPT]
- max_eval_cycles (기본 3)
- max_design_revisions (기본 3)
- 브랜치 보호 (main/master 직접 커밋 차단)
- 커밋 크기 제한
- 참고: buddy의 `guard-destructive-commands`, `freeze-edit-scope`

### COMMON-4. 로깅 체계 [NEW]
- 각 에이전트별 로그 파일 ({workspace}/logs/)
- 구조화된 로그 포맷
- failure 상세 로그

---

## 작업 통계

| Phase | NEW | ADAPT | REF | INFRA | 합계 |
|-------|-----|-------|-----|-------|------|
| Phase 1 | 6 | 0 | 0 | 3 | 9 |
| Phase 2 | 7 | 0 | 0 | 0 | 7 |
| Phase 3 | 9 | 1 | 0 | 0 | 10 |
| Phase 4 | 9 | 0 | 0 | 0 | 9 |
| Phase 5 | 3 | 5 | 0 | 0 | 8 |
| Phase 6 | 3 | 4 | 0 | 0 | 7 |
| Phase 7 | 2 | 4 | 0 | 0 | 6 |
| 공통 | 2 | 1 | 0 | 0 | 3 |
| **합계** | **41** | **15** | **0** | **3** | **59** |

## 구현 순서

Phase 번호 순서대로 진행. Phase 내부에서는 P{N}-{M} 순서대로.
단, 의존 관계:
- P1 완료 → P2~P7 진행 가능
- P2 완료 → P5 (Jira 읽기 필요)
- P3 + P4 완료 → P5 (CKS 검색 필요)
- P5 완료 → P6 (구현 코드 필요)
- P6 완료 → P7 (테스트 통과 필요)
- COMMON은 해당 Phase 진행 시 함께 구현
