# Phase 2: Jira Gateway MCP + Sensitive Filter — 작업 상세

> 설계 문서: [phase2-jira-gateway-mcp-sensitive-filter.md](../superpowers/specs/phase2-jira-gateway-mcp-sensitive-filter.md)

---

## P2-1. Jira Gateway MCP 서버 프로젝트 생성 [NEW] `M`

**파일**: `tools/jira-gateway-mcp/` 전체

**입력**: 없음 (프로젝트 스캐폴딩)

**산출물**:
```
tools/jira-gateway-mcp/
├── package.json          # name: @coding-agent/jira-gateway-mcp
├── tsconfig.json         # strict, ESM
├── src/
│   ├── index.ts          # MCP server 진입점 (stdio transport)
│   ├── server.ts         # tool 등록 + 라우팅
│   ├── tools/            # P2-5에서 구현
│   ├── filter/           # P2-3에서 구현
│   ├── upstream/         # P2-2에서 구현
│   └── types.ts          # 공유 타입
├── tests/                # P2-6에서 구현
└── .env.example          # JIRA_BASE_URL, JIRA_API_TOKEN, JIRA_USER_EMAIL
```

> ⚠️ **RI-22**: shared/patterns.json 접근 — TypeScript에서는 런타임 fs.readFile로
> `../../shared/patterns.json` 상대 경로 로드. 또는 .mcp.json의 env에서
> `PATTERNS_PATH` 환경변수로 경로 주입 (권장).

> ⚠️ **RI-01 + RI-15**: 이 Phase 시작 시 `.mcp.json`을 프로젝트 루트에 생성하여
> jira-gateway MCP 서버를 등록해야 한다. 환경변수(JIRA_BASE_URL 등)도 함께 설정.

**핵심 로직**:
```typescript
// src/index.ts
import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { registerTools } from "./server.js";

const server = new Server({ name: "jira-gateway", version: "0.1.0" }, {
  capabilities: { tools: {} }
});

registerTools(server);

const transport = new StdioServerTransport();
await server.connect(transport);
```

**완료 기준**:
- [ ] `npx tsx src/index.ts`로 MCP 서버 시작
- [ ] MCP Inspector 또는 Claude Code에서 tool 목록 조회 가능

---

## P2-2. Jira REST API 클라이언트 [NEW] `M`

**파일**: `tools/jira-gateway-mcp/src/upstream/jira-client.ts`

**입력**: 환경변수 (JIRA_BASE_URL, JIRA_API_TOKEN, JIRA_USER_EMAIL)

**산출물**: JiraClient 클래스

**핵심 로직**:
```typescript
class JiraClient {
  private baseUrl: string;
  private authHeader: string; // Basic base64(email:token)

  async getIssue(ticketId: string): Promise<JiraIssue>
  // GET /rest/api/3/issue/{ticketId}?fields=summary,description,status,assignee,labels,created,updated,comment

  async getComments(ticketId: string, since?: string): Promise<JiraComment[]>
  // GET /rest/api/3/issue/{ticketId}/comment

  async addComment(ticketId: string, body: string): Promise<void>
  // POST /rest/api/3/issue/{ticketId}/comment

  async transitionIssue(ticketId: string, transitionName: string): Promise<void>
  // 1. GET /rest/api/3/issue/{ticketId}/transitions → transition ID 조회
  // 2. POST /rest/api/3/issue/{ticketId}/transitions

  async searchIssues(jql: string, maxResults?: number): Promise<JiraIssue[]>
  // GET /rest/api/3/search?jql={jql}&maxResults={n}
}
```

**에러 처리**:
- 401 → "Jira 인증 실패. JIRA_API_TOKEN 확인"
- 404 → "티켓 {id}를 찾을 수 없습니다"
- 429 → 지수 백오프 재시도 (최대 3회, 초기 1s, 2x 증가)
- 네트워크 → 재시도 3회 후 실패

**완료 기준**:
- [ ] 환경변수 미설정 시 명확한 에러 메시지
- [ ] getIssue, getComments, addComment, transitionIssue, searchIssues 동작
- [ ] 429 rate limit 시 지수 백오프 재시도
- [ ] Jira Cloud API v3 호환

---

## P2-3. Sensitive Filter 엔진 [NEW] `L`

**파일**: `tools/jira-gateway-mcp/src/filter/`

**입력**: 텍스트 (Jira 응답 본문)

**산출물**: 필터링된 텍스트 + FilterMetadata

**핵심 로직**:

### engine.ts — 메인 엔진
```typescript
interface FilterResult {
  text: string;                    // 필터링된 텍스트
  metadata: {
    scan_result: "CLEAN" | "REDACTED" | "BLOCKED";
    redacted_count: number;
    redacted_patterns: string[];   // 패턴 ID만 (값 노출 안 함)
    blocked_patterns: string[];
    warnings: string[];
    scanned_at: string;
  };
}

function scanAndFilter(text: string, patterns: Pattern[]): FilterResult {
  // 1. regex 패턴 매칭
  regexMatches = patterns.filter(p => p.type !== "entropy")
    .flatMap(p => matchAll(text, p))

  // 2. 엔트로피 스캔
  entropyMatches = scanEntropy(text, entropyPattern)

  // 3. 모든 매치 합산, severity 판정
  allMatches = [...regexMatches, ...entropyMatches]
  
  // 4. block 판정: critical + action:block 존재 → BLOCKED
  if (allMatches.some(m => m.action === "block"))
    return blocked(allMatches)
  
  // 5. redact: action:redact 매치를 [REDACTED:{id}]로 치환
  // 6. warn: action:warn은 metadata에만 기록, 텍스트는 통과
  return redactAndWarn(text, allMatches)
}
```

### patterns.ts — 패턴 로더
```typescript
function loadPatterns(): Pattern[] {
  // 1. shared/patterns.json 로드 (기본)
  // 2. .coding-agent/custom-patterns.json 로드 (있으면)
  // 3. merge: 동일 id → custom override, 새 id → 추가
  // 4. regex 패턴 사전 컴파일 (RegExp 캐시)
}
```

### entropy.ts — Shannon entropy
```typescript
function shannonEntropy(text: string): number {
  // 문자 빈도 계산 → -Σ(p * log2(p))
}

function scanEntropy(text: string, config: EntropyConfig): Match[] {
  // 1. 공백/구분자로 토큰 분리
  // 2. min_length ~ max_length 범위 필터
  // 3. exclude_patterns 매칭 제외 (hex hash, 상수, URL)
  // 4. entropy > threshold → Match 생성
}
```

### redactor.ts — 치환 처리
```typescript
function redact(text: string, matches: Match[]): string {
  // position 역순 정렬 (뒤에서부터 치환해야 위치 불변)
  // 각 match → [REDACTED:{pattern_id}] 치환
}

function blocked(matches: Match[]): FilterResult {
  // 전체 텍스트 차단
  // error 메시지 + 탐지된 패턴 목록 반환
  // 실제 값은 절대 노출하지 않음
}
```

**fail-safe**: 필터 엔진 자체가 예외 발생 시 → 데이터를 통과시키지 않고 차단 + 에러 로그

**buddy 참고**:
- `plugin/skills/audit-security/PROCEDURE.md` — 보안 패턴 체크리스트
- `plugin/skills/design-secret-management/PROCEDURE.md` — 시크릿 탐지 패턴

**완료 기준**:
- [ ] regex 패턴 14개 전부 매칭 검증
- [ ] Shannon entropy 계산이 정확 (알려진 입력에 대해 예상값 일치)
- [ ] REDACT: 매치 부분만 정확히 치환, 나머지 텍스트 보존
- [ ] BLOCK: critical+block 패턴 시 전체 차단
- [ ] WARN: 텍스트 통과 + metadata에 경고 기록
- [ ] fail-safe: engine 예외 시 차단 (통과 아님)
- [ ] exclude_patterns로 오탐 방지 (hex hash, URL 등)

---

## P2-4. patterns.json [NEW] `S`

**상태**: ✅ 완료

`shared/patterns.json`에 14개 패턴 정의 완료.
커스텀 패턴 merge 로직은 P2-7에서 구현.

---

## P2-5. MCP Tool 등록 [NEW] `M`

**파일**: `tools/jira-gateway-mcp/src/tools/`

**핵심 로직**:

### 읽기 tools (필터 적용)
```typescript
// read-ticket.ts
server.setRequestHandler(CallToolRequestSchema, async (request) => {
  if (request.params.name === "jira_read_ticket") {
    const { ticket_id } = request.params.arguments;
    
    // 1. Jira API 호출
    const raw = await jiraClient.getIssue(ticket_id);
    
    // 2. description + summary + comments를 필터 엔진에 통과
    const descResult = scanAndFilter(raw.description, patterns);
    const summaryResult = scanAndFilter(raw.summary, patterns);
    
    // 3. BLOCKED 판정 시 → 에러 응답
    if (descResult.metadata.scan_result === "BLOCKED") {
      return { content: [{ type: "text", text: JSON.stringify({
        error: "SENSITIVE_CONTENT_BLOCKED",
        detected_patterns: descResult.metadata.blocked_patterns
      })}]};
    }
    
    // 4. 정상 → 필터링된 데이터 + _filter_metadata 반환
    return { content: [{ type: "text", text: JSON.stringify({
      ticket_id, type: raw.issuetype,
      summary: summaryResult.text,
      description: descResult.text,
      ...
      _filter_metadata: mergeMetadata(summaryResult.metadata, descResult.metadata)
    })}]};
  }
});
```

### 쓰기 tools (passthrough)
```typescript
// add-comment.ts — Jira API 직접 전달, 필터 없음
// update-status.ts — transitionIssue 호출
// update-assignee.ts — 필드 업데이트
```

**완료 기준**:
- [ ] 6개 tool 전부 MCP Inspector에서 호출 가능
- [ ] 읽기 tool 응답에 _filter_metadata 포함
- [ ] BLOCKED 시 에러 응답 (원본 데이터 미전달)
- [ ] 쓰기 tool은 필터 없이 직접 전달

---

## P2-6. 필터 단위 테스트 [NEW] `M`

**파일**: `tools/jira-gateway-mcp/tests/`

**테스트 케이스**:
```
filter.test.ts:
  - PEM private key → BLOCKED
  - AWS access key in text → REDACTED (해당 부분만)
  - JWT token → REDACTED
  - 일반 텍스트 → CLEAN
  - 복수 패턴 동시 탐지 → 모든 매치 처리
  - 빈 텍스트 → CLEAN

entropy.test.ts:
  - "aB3$kL9#mN2&pQ5" (고엔트로피) → 탐지
  - "aaaaaaaaaaaaaaaaaaaaaa" (저엔트로피) → 미탐지
  - "0123456789abcdef0123" (hex hash) → exclude로 미탐지
  - "https://example.com/path" (URL) → exclude로 미탐지

redactor.test.ts:
  - 단일 매치 REDACT → 정확한 위치 치환
  - 복수 매치 REDACT → 모든 매치 치환, 위치 정확
  - BLOCK → 전체 텍스트 미반환
  - fail-safe → 엔진 예외 시 차단
```

**완료 기준**:
- [ ] 모든 테스트 PASS
- [ ] 엣지 케이스: 패턴이 텍스트 시작/끝에 위치, 중첩 패턴

---

## P2-7. 패턴 커스터마이징 [NEW] `S`

**파일**: patterns.ts의 loadPatterns() 내부

**핵심 로직**:
```typescript
function loadPatterns(): Pattern[] {
  const base = JSON.parse(readFile("shared/patterns.json"));
  const customPath = ".coding-agent/custom-patterns.json";
  
  if (!exists(customPath)) return base.patterns;
  
  const custom = JSON.parse(readFile(customPath));
  
  // merge: id 기준
  const merged = new Map(base.patterns.map(p => [p.id, p]));
  for (const p of custom.patterns) {
    merged.set(p.id, p); // 동일 id → override
  }
  return Array.from(merged.values());
}
```

**완료 기준**:
- [ ] custom-patterns.json 미존재 시 기본 패턴만 로드
- [ ] 동일 id → custom이 override
- [ ] 새 id → 추가
