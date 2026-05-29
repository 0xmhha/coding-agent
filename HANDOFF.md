# Coding Agent — Cross-Session Handoff

> 다른 머신/다른 세션에서 작업을 이어받기 위한 자기완결적 컨텍스트 문서.
> 이 저장소를 clone 한 새 세션이 이 문서만으로 같은 목적·같은 규칙으로 작업을 계속할 수 있도록 작성됨.

| 항목 | 값 |
|------|----|
| 작성일 | 2026-05-29 |
| 마지막 커밋 | `2d9cec6 docs: mark all 23 review issues resolved with audit summary` |
| 브랜치 | `main` (origin보다 3 commits 앞섬, push 안 됨) |
| 작업 진행률 | Phase 1~7 + RI-01~23 + SETUP.md 모두 완료. 다음 단계는 §7 "남은 작업" 참조 |

---

## 1. 1분 요약 (TL;DR)

**프로젝트 정체성**: `go-stablenet` 전용 Claude Code plugin. Jira 티켓을 입력으로 받아 자동으로 분석→설계→구현→테스트→PR→리뷰 반영→merge 까지 수행하는 다중 에이전트 파이프라인.

**대상 코드베이스**: `go-stablenet` — geth fork. WBFT consensus, ETH 대신 stable coin native, system contracts 다수.

**기술 스택**: Claude Code Plugin (auto-discovery) + Go MCP servers (CKV/CKG + Jira Gateway) + Bash hooks + SQLite (modernc.org/sqlite, CGo-free) + 선택적 Ollama (nomic-embed-text).

**핵심 보안 원칙**: 민감 정보(시크릿/사내 정보)는 LLM에 도달하기 전에 차단해야 한다 → 모든 외부 데이터는 **Proxy MCP Gateway** 패턴으로 필터링.

**현재 상태**: 사양 + 구현 + RI 감사 + 셋업 가이드까지 완료. 실 환경 실증과 운영성(README, CI, 릴리즈)이 남음.

---

## 2. 사용자 규칙 (Critical — 반드시 준수)

이 규칙은 글로벌 CLAUDE.md(`~/.claude/CLAUDE.md`)에 기록된 사용자 명시 선호. 새 머신에도 동일 글로벌 설정이 있다는 가정. 없다면 이 문서를 글로벌 룰의 일부로 취급할 것.

1. **사용자가 한국어로 메시지를 보내면 한국어로 응답.** 영어로 보내면 영어로 응답.
2. **Git commit 메시지는 영어로 작성, 간결하게.**
3. **Commit 메시지에 `Co-Authored-By:` 또는 "Generated with [Claude Code]" attribution 절대 포함하지 않음.**
4. **작업 종료 시 uncommitted 변경사항이 있으면 사용자에게 커밋 여부를 먼저 확인.** 자율 커밋·자율 폐기 금지.
5. **파괴적 git 작업(force push, reset --hard, branch -D)은 사용자 명시 승인 없이 절대 수행하지 않음.**
6. **Reflect-Verify-Fix 프로토콜**: 코드 수정/명령 실행 직후 검증을 *별도 스텝*으로. 3회 연속 동일 오류 또는 2회 수정 후에도 실패하면 중단하고 사용자에게 보고.
7. **민감 정보(secrets/PII)는 grep으로 확인 가능 — 커밋 전 항상 점검.**

---

## 3. 시스템 개요

### 3.1 무엇을 자동화하는가

```
Jira 티켓 (STABLE-xxxx)
  │
  ▼
TICKET_INTAKE → ANALYSIS → PLANNING → DESIGN → IMPLEMENTATION → EVALUATION → COMPLETION
  │              │           │          │        │                │              │
  │              │           │          │        │                │              ▼
  │              │           │          │        │                │           PR + Jira 댓글 + 상태 전이
  │              │           │          │        │                │
  │              │           │          │        │                ▼
  │              │           │          │        │           Pass/Fail → fail 시 사이클 (max 3회) 또는 BLOCKED
  │              │           │          │        ▼
  │              │           │          │     단일/원자 step 단위 커밋
  │              │           │          ▼
  │              │           │       design-v{N}.md (self-review, max revisions 3)
  │              │           ▼
  │              │        plan.md (atomic/reviewable/verifiable steps + verification plan)
  │              ▼
  │           analysis.md + related-code.json (CKV + CKG + stablenet-context)
  ▼
ticket.json (Jira description ADF→Markdown + 민감정보 필터링)
```

**별도 진입점**:
- `/review <PR#>`: PR 코멘트 수집 → 7유형 × 4심각도 분류 → review-feedback-{N}.md → bugfix 모드로 ANALYSIS 재진입
- `/merge <PR#>`: 5단계 전제조건 검증 → squash merge → Jira 완료 처리
- `/status [JIRA-ID]`: 활성 워크스페이스 또는 단일 티켓 상세 확인

**파이프라인 변종 (RI-18, RI-19)**:
- `feature` (기본): 위의 풀 사이클
- `code_review`: TICKET_INTAKE → ANALYSIS → PLANNING(review-report.md) → COMPLETION
- `release`: TICKET_INTAKE → ANALYSIS → EVALUATION → COMPLETION(git tag + CHANGELOG)
- `bugfix` (review cycle 재진입): PLANNING(plan-fix-{N}.md) → DESIGN → IMPLEMENTATION

### 3.2 아키텍처: B+C 하이브리드

여러 후보 중 사용자가 **B+C 하이브리드** 선택. 의미:
- **B**: Document-driven 상태 머신 (state.json + analysis.md/plan.md/design-v{N}.md/test-report.md를 파일로 보존). 컨텍스트가 truncate 되어도 다음 세션이 파일에서 복원.
- **C**: Multi-agent (Orchestrator + Planner + Implementer + Evaluator). 각 에이전트는 격리 컨텍스트로 dispatch. Orchestrator만 전체 흐름을 본다.

### 3.3 보안 모델: Proxy MCP Gateway

**핵심 결정의 Why**:
- 처음에는 sensitive-check skill로 LLM이 정보를 보고 판단하는 안을 검토. 사용자가 "Skill로 처리하면 정보가 이미 LLM에 도달한 순간 노출이다"라고 지적.
- 결론: **MCP server level**에서 LLM에 도달하기 전에 필터링해야 함. 모든 외부 데이터(Jira description/comments, search results)는 `tools/jira-gateway-mcp/internal/filter`가 정규식 + 엔트로피 + 화이트리스트로 스캔해 REDACTED/BLOCKED 결정 후 sanitized 텍스트만 LLM에 노출.
- **Outbound도 대칭으로 처리**: PR body, squash commit body, Jira 댓글 — `plugin/skills/pr-sanitize/SKILL.md`가 동일 `shared/patterns.json`을 적용.

---

## 4. 저장소 레이아웃

```
coding-agent/
├── HANDOFF.md                       ← 이 문서. 새 세션 진입점
├── go.work                          ← 두 Go module 통합
├── shared/
│   └── patterns.json                ← 14개 민감정보 패턴 (CRITICAL/HIGH/MEDIUM + entropy)
├── plugin/                          ← Claude Code Plugin 본체
│   ├── .claude-plugin/plugin.json   ← Plugin manifest
│   ├── .mcp.json                    ← jira-gateway / cks 서버 등록
│   ├── commands/
│   │   ├── work.md                  ← /work — 메인 진입점 (--local 지원)
│   │   ├── review.md                ← /review — PR 코멘트 수집/분류
│   │   ├── status.md                ← /status — 활성/단일 티켓 상태
│   │   └── merge.md                 ← /merge — squash merge + post-merge
│   ├── agents/
│   │   ├── orchestrator.md          ← 상태 전이 dispatch + 파이프라인 변종 분기
│   │   ├── planner.md               ← ANALYSIS/PLANNING/DESIGN (4 modes)
│   │   ├── implementer.md           ← 브랜치 관리 + per-step 구현 + checkpoint
│   │   └── evaluator.md             ← 4-stage 검증 (unit/lint/security/chainbench)
│   ├── skills/
│   │   ├── state-machine/SKILL.md   ← state.json 읽기/쓰기/get_resume_point/transition guards
│   │   ├── template-parse/SKILL.md  ← 4가지 work-type 파싱 (ADF 참고 주석 포함)
│   │   ├── stablenet-context/SKILL.md ← 도메인 분류 + 복잡도 + impact graph
│   │   └── pr-sanitize/SKILL.md     ← outbound 민감정보 스크러버
│   └── hooks/
│       ├── hooks.json               ← PostToolUse 매핑
│       ├── on-agent-complete.sh     ← 서브에이전트 종료 로깅 (fail-open)
│       └── on-commit.sh             ← 커밋 hash/subject/stat을 impl.log 기록
├── tools/                           ← MCP 서버 실제 구현 (Go)
│   ├── jira-gateway-mcp/            ← Phase 2
│   │   ├── go.mod (modernc-free)
│   │   ├── cmd/server/main.go
│   │   └── internal/
│   │       ├── jira/                 ← client.go, adf.go, adf_test.go
│   │       ├── filter/               ← engine.go, patterns.go, entropy.go, redactor.go + tests
│   │       ├── server/server.go      ← 6개 tool 등록 (3 read + 3 write)
│   │       └── types/types.go
│   └── cks-mcp/                     ← Phase 3+4
│       ├── go.mod
│       ├── cmd/server/main.go        ← CKV+CKG 공유 SQLite 경로
│       └── internal/
│           ├── ckv/                  ← chunker, embedder, store, bm25, reranker, search, indexer
│           ├── ckg/                  ← relations(Tier1/Tier2), history, concurrency, store, traversal, query, indexer
│           ├── filter/               ← sensitive filter (jira-gateway 포팅, CKS_PATTERNS_PATH 우선)
│           ├── server/server.go      ← 5개 tool (ckv_search, ckv_index, ckg_query, ckg_impact, ckg_index)
│           └── types/types.go
└── docs/
    ├── SETUP.md                     ← 10-section 설치/실행 가이드 (RI-16)
    ├── superpowers/specs/           ← 8개 설계 문서
    │   ├── 2026-05-27-coding-agent-plugin-design.md   ← 전체 시스템 설계
    │   ├── phase1-plugin-skeleton-state-machine.md
    │   ├── phase2-jira-gateway-mcp-sensitive-filter.md
    │   ├── phase3-cks-mcp-ckv.md
    │   ├── phase4-cks-mcp-ckg.md
    │   ├── phase5-agent-pipeline.md
    │   ├── phase6-evaluator-chainbench.md
    │   └── phase7-pr-review-cycle.md
    └── plan/
        ├── WORK_BREAKDOWN.md        ← Phase별 작업 수 + 통계
        ├── REVIEW_ISSUES.md         ← 23개 RI + 최종 감사 (2026-05-29 모두 RESOLVED)
        ├── common-tasks.md
        └── phase{1..7}-tasks.md
```

---

## 5. 설계 결정과 Why

새 세션이 임의로 뒤집지 말아야 할 결정들. 각 항목은 *왜* 그렇게 했는지를 기록.

| 결정 | Why | 위치 |
|------|-----|------|
| **MCP 서버 언어 = Go** | 처음 TypeScript로 시작 → 사용자가 "다른 TS 사용처 없는데 Go로 충분하지 않냐"고 지적. Go는 단일 바이너리, CGo-free, 빠른 시작, geth 생태계와 동일 언어 | `tools/*/go.mod` |
| **B+C 하이브리드 아키텍처** | A(B-only)는 LLM 호출 분리 어려움, C(C-only)는 컨텍스트 손실 시 복원 불가. 둘 다 채택 — 상태는 파일, 실행은 에이전트 격리 | `docs/superpowers/specs/2026-05-27-*.md` §2 |
| **Proxy MCP Gateway 패턴** | Sensitive check skill로 처리 시 이미 LLM에 데이터가 도달 → 무용. MCP 서버 내부에서 outbound 전에 filter. | `tools/jira-gateway-mcp/internal/filter/` |
| **modernc.org/sqlite (CGo-free)** | sqlite-vss는 C 확장이라 CGo 강제. 크로스 플랫폼 빌드 복잡. brute-force cosine for ~20K chunks × 768d = 10–50ms이므로 MVP 충분 | `tools/cks-mcp/internal/ckv/store.go:17` 주석 |
| **BM25 폴백 (Tier 2)** | Ollama 미설치 환경에서도 CKV 사용 가능하도록. `engine` 필드로 클라이언트가 fallback 사용 여부 식별 | `tools/cks-mcp/internal/ckv/bm25.go` |
| **Tier-1 typed + Tier-2 AST-only 폴백** | `packages.Load` 실패 (빌드 환경 미비) 시에도 관계 추출 가능하도록. confidence 등급으로 신뢰도 표면화 | `tools/cks-mcp/internal/ckg/relations.go:27` |
| **code_hash 캐시** | incremental indexing 시 변경 안 된 청크는 임베딩 재계산 스킵 → Ollama 호출 비용 절감 | `tools/cks-mcp/internal/ckv/store.go` chunks.code_hash 컬럼 |
| **ADF→Markdown 자체 구현 (Option B)** | Option A(HTML 경유)는 HTML 변환 라이브러리 추가 의존. Option B(ADF 직접 파싱)가 의존성 그래프 가벼움 | `tools/jira-gateway-mcp/internal/jira/adf.go` |
| **transition 3-tier lookup (RI-05)** | Jira workflow의 transition name은 프로젝트별로 다름. name → status name → statusCategory key 순으로 case-insensitive 매칭 → 별도 설정 파일 없이 흡수 | `tools/jira-gateway-mcp/internal/jira/client.go:140 TransitionIssue` |
| **patterns.json 공유 = env 주입** | 빌드 시 embed 또는 symlink는 빌드 의존성 생성. env (`PATTERNS_PATH`, `CKS_PATTERNS_PATH`) 주입 + 폴백 경로 탐색이 가장 단순 | `plugin/.mcp.json` env block |
| **/merge body 2-tier 전략 (RI-14)** | step ≤10 → 전체 나열 / 11+ → [Interface, Implementation, Tests, Docs, Misc] 5-카테고리 버킷 | `plugin/commands/merge.md §4.2` |
| **commit 분할 (atomic / reviewable / verifiable)** | Planner가 step을 단일 책임 단위로 쪼개고, Implementer가 step당 1커밋. 리뷰어 시점에서 reasoning 추적 가능 | `plugin/agents/planner.md §4` + `implementer.md §4` |
| **race detector scope 제한 (RI-21)** | 전체 -race는 시간 폭발. CKG concurrency_impact에서 위험 패키지만 race_pkgs로 추출해 그쪽만 실행 | `plugin/agents/evaluator.md §4.4` |
| **release 변종에서 tag/push는 사용자 확인 게이트** | 자동 태깅·푸시는 되돌리기 어려움. orchestrator에 "Never tag or push tags without user confirmation" 명시 | `plugin/agents/orchestrator.md §6 safety policies` |

---

## 6. 완료한 작업 (검증된 사실)

### 6.1 Phase 단위

| Phase | 작업 수 | 상태 | 주요 산출물 |
|-------|---------|------|-----------|
| Phase 1 (Skeleton + State Machine) | 10 | ✅ | plugin manifest, 4 commands, state-machine/template-parse/stablenet-context skills, hook 골격 |
| Phase 2 (Jira Gateway MCP) | 7 | ✅ | Go MCP server, ADF 변환, 6 tools, sensitive filter + fail-safe |
| Phase 3 (CKV) | 10 | ✅ | Go AST chunker, Ollama probe, BM25, brute-force cosine, reranker, indexer |
| Phase 4 (CKG) | 9 | ✅ | 7 relation types, Tier 1/2 fallback, history, concurrency, traversal, impact |
| Phase 5 (Agent Pipeline) | 9 | ✅ | Orchestrator + Planner (4 modes) + Implementer + checkpoint + hooks |
| Phase 6 (Evaluator) | 7 | ✅ | 4-stage (unit+race / lint / security / chainbench) + test-report + failure_log |
| Phase 7 (PR + Review + Merge) | 7 | ✅ | PR body 조립, pr-sanitize, /review 7×4 분류, /merge 5-precondition |
| 공통 | 4 | ✅ (Q-1/Q-2 부분만 분리 가능) | patterns.json, workspace 패턴, safeguards, evaluator logs |

### 6.2 Review Issue 감사

`docs/plan/REVIEW_ISSUES.md` §"최종 감사 (2026-05-29)" 참조. 23개 모두 `RESOLVED` + 증거 (코드 경로/커밋 hash) 기록.

런타임 실증 잔존 (사용자 환경 종속):
- **RI-20**: 실제 ChainBench MCP tool 이름은 §7.0 pre-flight가 첫 실행 시 자동 검증·BLOCKED
- **RI-08, RI-09**: Ollama 가용성·인덱싱 시간은 호스트 자원 의존. BM25 폴백 + code_hash 캐시로 worst-case 흡수

### 6.3 SETUP.md (RI-16)

10개 섹션. 새 머신에서 실행 시 첫 참조 문서: prerequisites → clone → build → env vars → plugin install → 첫 인덱싱 → smoke test (--local) → real workflow → troubleshooting → next reading.

### 6.4 통합 검증 발견 사항 (해결됨)

- **MCP go-sdk v1.6.1 jsonschema 태그 panic**: `jsonschema:"required,description=..."` 형식이 panic. v1.6.1은 `jsonschema:"description text"` 형식만 허용 (required는 omitempty 부재로 암시). 두 server.go 모두 수정. 커밋 `ebfe5eb`.

---

## 7. 남은 작업 (Roadmap)

분류는 우선순위가 아니라 작업 성격. §7.5에 권장 순서.

### 7.1 A. 품질 향상 (선택적)

| ID | 작업 | 범위 | 차단 요인 |
|----|------|------|---------|
| Q-1 | `workspace-helper` skill 추출 | 현재 `/work`, `/status`, `/review`, `/merge`에 4중 복제된 `.coding-agent/tickets/{id}_*` 패턴을 단일 skill로 추출 | 없음. 순수 리팩토링 |
| Q-2 | 에이전트별 통일 로깅 | 현재 evaluator만 `{ws}/logs/eval-*.log` 출력. orchestrator/planner/implementer도 `{ws}/logs/{agent}.log` 패턴 적용 | 없음. SKILL/agent 문서 수정만 |

### 7.2 B. 런타임 실증

| ID | 작업 | 범위 | 차단 요인 |
|----|------|------|---------|
| R-1 | 실 Jira 인스턴스 E2E | jira-gateway MCP 실 호출 + ADF 다양 노드(color/inline card/mention)에서 누락 케이스 확인 | JIRA_BASE_URL + JIRA_API_TOKEN + 실 STABLE-xxxx 티켓 |
| R-2 | go-stablenet full indexing | CKV full index 시간 측정, BM25 폴백 정확도 검증, CKG relations 추출 신뢰도 확인 | go-stablenet 로컬 clone + (선택) Ollama |
| R-3 | ChainBench MCP §7.0 pre-flight 실측 | 실제 tool 이름과 spec의 [chainbench_setup, chainbench_start, chainbench_status, chainbench_run_tests, chainbench_stop] 일치 검증. 불일치 시 evaluator.md §7.0 fallback 메시지에 spec 보정 안내 | ChainBench MCP 인스턴스 |
| R-4 | /work → /review → /merge 풀 사이클 1회 | 단일 STABLE 티켓으로 end-to-end. 누락된 엣지 케이스 노출 | 실 Jira + go-stablenet PR 권한 + ChainBench |

### 7.3 C. 운영성 / 배포

| ID | 작업 | 범위 | 차단 요인 |
|----|------|------|---------|
| O-1 | `README.md` | 프로젝트 1페이지 요약. SETUP.md는 절차서이므로 별도 | 없음 |
| O-2 | `LICENSE` | Apache-2.0 또는 MIT 권장 (사용자 확인 필요) | 사용자 선택 |
| O-3 | GitHub Actions CI | 두 MCP 서버 빌드 + `go test ./...` + `go vet` | 없음 |
| O-4 | v0.1.0 첫 릴리즈 | 빌드된 두 바이너리를 Releases에 첨부 → SETUP.md "Build from source" 스킵 가능 | O-3 통과 후 |

### 7.4 D. 후속 확장 (Out of scope, 후순위)

- 다중 ticket 동시 처리 (현재 single workspace 락 가정)
- GitLab/Gitea 지원 (현재 `gh` CLI/GitHub PR API 종속)
- 다른 geth fork 지원 (stablenet-context skill 일반화)

### 7.5 권장 진행 순서

가장 가치 높은 순:
1. **O-1 (README.md)** — 1시간 미만. 현 상태 외부 공유 가능 베이스라인
2. **O-3 (CI)** — 회귀 방지. 후속 변경 안전망
3. **Q-1 (workspace-helper)** — 코드 중복 제거. 후속 명령 추가 비용 절감
4. **R-2 (실 코드 인덱싱)** — RI-09 시간 추정 보정 + BM25 정확도 실측
5. **O-4 (v0.1.0)** — 사용자 배포 채널 확보
6. **R-1, R-3, R-4** — 실 환경 의존, 사용자가 환경 준비된 시점에 진행

---

## 8. 다음 세션 시작 가이드

### 8.1 최소 읽기 순서 (15분 안에 컨텍스트 확보)

1. **이 문서** (HANDOFF.md) — 전체 그림
2. **`docs/SETUP.md`** — 실행/빌드/디버깅 방법
3. **`docs/superpowers/specs/2026-05-27-coding-agent-plugin-design.md`** — 시스템 설계 원본
4. **`docs/plan/REVIEW_ISSUES.md`** — 모든 의사결정 근거 + 트레이드오프
5. **`plugin/agents/orchestrator.md`** — 상태 전이의 진실 소스 (state machine 그림)

세부 작업 진입 시:
- 에이전트 사양 변경 → `plugin/agents/*.md` + 해당 Phase spec
- MCP 서버 변경 → `tools/{jira-gateway-mcp,cks-mcp}/internal/server/server.go`
- 새 RI 발견 → `docs/plan/REVIEW_ISSUES.md`에 RI-24 형태로 추가

### 8.2 환경 재구축 절차

```bash
# 1. clone
git clone <repo-url> coding-agent && cd coding-agent

# 2. Build MCP binaries
cd tools/jira-gateway-mcp && go build -o bin/jira-gateway-mcp ./cmd/server && cd -
cd tools/cks-mcp           && go build -o bin/cks-server        ./cmd/server && cd -

# 3. Test
cd tools/jira-gateway-mcp && go test ./... && cd -
cd tools/cks-mcp           && go test ./... && cd -

# 4. Plugin install — SETUP.md §5 참조
```

### 8.3 새 세션 첫 prompt 예시

> 이 저장소 루트의 `HANDOFF.md`를 먼저 읽고 §7.5 권장 순서대로 진행해줘. 작업 시작 전 §2 사용자 규칙 준수 여부를 확인하고, uncommitted 변경사항이 있으면 커밋 여부를 먼저 물어봐.

### 8.4 멘탈 모델 점검

새 세션이 다음 질문에 답할 수 있어야 함:
- Q: 왜 MCP 서버가 두 개인가?
  - A: 책임 분리. jira-gateway = inbound 외부 데이터 필터링, cks = 코드 지식 (CKV semantic + CKG structural).
- Q: 왜 LLM이 patterns.json을 직접 안 보는가?
  - A: §3.3 보안 모델. LLM에 도달하기 전에 차단해야 함.
- Q: state.json이 손상되면?
  - A: state-machine SKILL.md §2.6 get_resume_point — sub_status (fresh/partial/review_pending) 복구. 아티팩트 파일(analysis.md/plan.md/design-v{N}.md)이 있으면 그쪽이 우선.
- Q: BM25 폴백은 언제 동작하나?
  - A: `OLLAMA_BASE_URL` 미설정 또는 `CKS_DISABLE_OLLAMA=1` 또는 임베딩 호출 실패. 응답의 `engine` 필드로 식별.

---

## 9. 환경 변수 / 외부 의존성

### 9.1 필수

| 변수 | 용도 | 비고 |
|------|------|------|
| `JIRA_BASE_URL` | `https://yourorg.atlassian.net` | jira-gateway-mcp |
| `JIRA_API_TOKEN` | Atlassian API token | 절대 커밋 금지 |
| `JIRA_USER_EMAIL` | API 토큰 발급 계정 | |
| `CKS_INDEX_PATH` | SQLite 인덱스 파일 절대 경로 | CKV + CKG 공유 |

### 9.2 선택 (있으면 동작 모드 변경)

| 변수 | 효과 |
|------|------|
| `OLLAMA_BASE_URL` (예: `http://localhost:11434`) | Ollama 임베딩 사용. 미설정 시 BM25 폴백 |
| `OLLAMA_EMBED_MODEL` (예: `nomic-embed-text`) | 임베딩 모델 |
| `CKS_DISABLE_OLLAMA=1` | 강제 BM25 모드 |
| `PATTERNS_PATH` | jira-gateway-mcp의 patterns.json override |
| `CKS_PATTERNS_PATH` | cks-mcp의 patterns.json override (PATTERNS_PATH보다 우선) |
| `CUSTOM_PATTERNS_PATH` | 사용자 커스텀 패턴 merge |

### 9.3 외부 도구

- `go` 1.22+ (modernc/x/tools/go/packages 호환)
- `git` (history analyzer가 `git log -L` 호출)
- `gh` CLI (PR 생성/병합)
- (선택) `golangci-lint`, `gofmt`, `goimports`, `gosec`, `go vet` — evaluator stages
- (선택) `ollama` + `nomic-embed-text` — semantic embedding
- (필수, evaluator §7) ChainBench MCP — Stage 4 통합 테스트

---

## 10. 알려진 위험 / 미실증 가정

새 세션이 의심해야 할 가정들:

1. **ChainBench MCP의 tool 이름이 spec과 일치한다는 가정**: 실증 안 됨. §7.0 pre-flight가 자동 검증·BLOCKED 처리하므로 fail-safe.
2. **ADF 변환기가 모든 노드 타입을 커버한다는 가정**: 8+ 노드 타입 테스트했지만 color/inline card/mention/emoji 등 누락 가능. 실 Jira 인스턴스로 R-1 실증 필요.
3. **`packages.Load`가 go-stablenet 전체 트리에서 성공한다는 가정**: 실증 안 됨. 실패 시 Tier 2 폴백 자동 동작하지만 confidence 등급 저하.
4. **Ollama 임베딩 시간 추정 (RI-09)**: 청크당 200ms 기준 ~67분 (20K 청크). 실 측정 안 됨. CPU/GPU 자원에 크게 의존.
5. **/merge가 squash로만 동작한다는 가정**: merge commit / rebase 지원 안 함. 사용자 합의로 결정된 결정 — 뒤집지 말 것.
6. **state.json 구조의 후방 호환성**: 현재 스키마 변경 시 마이그레이션 로직 없음. 명시적으로 RI-2X로 등록되지 않은 상태. 큰 변경 전에 사용자 확인 필요.

---

## 11. 메모리 / 히스토리

이 저장소는 사용자의 글로벌 메모리 시스템(`~/.claude/projects/-Users-.../-coding-agent/memory/`)에 다음과 같이 등록되어 있음 (이전 머신 기준):

- `project_coding-agent.md` — go-stablenet 전용 Jira 기반 자동화 플러그인. B+C 하이브리드.
- `user_stablenet-dev.md` — 보안 민감, 설계 우선, 실패 복구 중시, 커밋 분할 선호.
- `feedback_commit-style.md` — English, concise, no co-author attribution.
- `reference_buddy-project.md` — `~/Work/github/study/ai/buddy`의 154 skill 컬렉션. 일부 패턴이 이 프로젝트에 적응됨 (auto-create-pr, finish-development-branch, iterate-fix-verify 등).

새 머신에서는 메모리가 없으므로 이 HANDOFF.md가 그 역할을 대체. 새 세션이 사용자 선호를 학습하면 글로벌 메모리에 동일 항목 저장 권장 (§2 규칙 4·5는 글로벌 CLAUDE.md에도 있어야 안전).

---

## 12. 변경 이력

| 날짜 | 커밋 | 변경 요약 |
|------|------|----------|
| 2026-05-27 | (다수) | Phase 1~7 설계 문서 작성 |
| (이어서) | scaffold + plugin 구조 정립 + tools/ 분리 |
| (이어서) | Phase 1~7 구현 (TypeScript → Go 전환 포함) |
| (이어서) | 통합 검증 + jsonschema 태그 fix |
| 2026-05-29 | `9c68bbf` | SETUP.md + RI-16 RESOLVED |
| 2026-05-29 | `2d9cec6` | 23개 RI 최종 감사 + 모두 RESOLVED |
| 2026-05-29 | (이 커밋) | HANDOFF.md 작성 |

---

**이 문서가 stale 되면 가장 먼저 update**: §1 마지막 커밋, §6 완료 작업, §7 남은 작업, §12 변경 이력.
