---
name: domain-pack
description: "프로젝트-불문 도메인팩 로더. state.json의 project_id로 활성 프로젝트의 domain-pack(${CLAUDE_PLUGIN_ROOT}/domains/{project_id}/)을 런타임에 해석해, 경로→모듈 분류·복잡도 추정·항상-켜진 invariants backstop을 제공한다. 도메인 *지식*은 그 프로젝트의 cks 인덱스가 권위. (ADR docs/adr/ADR-0001-domain-pack-contract.md)"
type: skill
---

# Domain-Pack Loader (generic — resolves the active project's domain pack)

이 스킬은 **프로젝트 고유 내용을 담지 않는다.** 절차(어떻게 분류·평가·backstop 적용하나)만
제너릭하게 정의하고, 데이터는 활성 프로젝트의 팩에서 런타임 `Read`로 끌어온다. 그래서 코어
에이전트는 `stablenet-*` 같은 프로젝트명을 호명하지 않고 이 스킬 하나만 참조한다.

> **도입 상태 (P1 Phase 1):** 이 로더는 신설됐으나 **아직 에이전트 frontmatter가 참조하지 않는다.**
> Phase 2에서 analyzer/planner/evaluator의 `skills:`가 `stablenet-{context,invariants}` →
> `domain-pack`으로 바뀌고, 본문 `stablenet-context.*` 호출이 아래 절차로 대체된다.

## 1. 활성 팩 해석 (런타임)

```
project_id = read {workspace_dir}/state.json → .project_id      # 예: "go-stablenet"
pack       = Read(${CLAUDE_PLUGIN_ROOT}/domains/{project_id}/domain-pack.json)  # 매니페스트
```

`project_id`가 없으면(구 워크스페이스) 기본값 `"go-stablenet"`으로 폴백하고 그 사실을 기록한다.
매니페스트나 디렉터리가 없으면 BLOCKED 사유로 보고한다(활성 팩 미설치).

> 메커니즘 근거: `Read`는 *데이터-의존 경로*를 런타임에 읽는 동작이라 frontmatter 정적 한계와
> 무관하다. 기존 패턴(state-machine의 state.json Read, 구 도메인 컨텍스트 스킬의 cks 라이브 호출)과
> 동일하다. (ADR §3.1)
> **경로 주의**: `${CLAUDE_PLUGIN_ROOT}`는 *설치된 플러그인 루트*(예: `~/.claude/plugins/cache/
> .../<version>/`)로 **로드 시점에 인라인 치환**된다(공식: skill/agent 본문에서 치환). 에이전트
> cwd는 *타깃 repo*라 `plugin/domains/...`·`domains/...` 같은 상대경로는 안 풀린다 — 번들 팩 파일은
> **반드시 `${CLAUDE_PLUGIN_ROOT}` 기준 절대경로**로 Read한다.

## 2. 제공 절차

### 2.1 classify_domain(file_paths, symbols)
`Read(${CLAUDE_PLUGIN_ROOT}/domains/{project_id}/{pack.context_classifier})` 의 경로→모듈 규칙(§"경로 기반 모듈
분류")으로 각 file_path를 분류 → 중복 제거 → 빈도순 정렬. 심볼이 모호하면 cks `find_symbol`로
경로를 얻어 같은 규칙 적용. 출력: `{primary_domain, domains[], confidence}`.

### 2.2 estimate_complexity(domains, change_summary)
같은 classifier 파일의 복잡도 휴리스틱(simple/moderate/complex + 동시성 키워드 승급)을 적용.
출력: `{complexity, reasoning}`.

### 2.3 invariants backstop (항상-켜짐)
`Read(${CLAUDE_PLUGIN_ROOT}/domains/{project_id}/{pack.invariants})` 의 불변식 목록을 **검색 결과와 무관하게**
적용한다(L3 backstop): Planner는 설계가 이를 위반하지 않게, Evaluator는 diff가 이를 깨지
않았는지 판정. 도메인별 정확한 수치·anchor의 권위는 그 프로젝트의 cks 엔트리다.

## 3. 경계

- 도메인 *지식*(불변식 수치·contract 이름·합의 규칙)은 이 스킬이나 팩 파일에 하드코딩하지 않는다 —
  활성 프로젝트의 cks 인덱스(`guidance.*` / 도메인 엔트리)가 권위. 팩은 *검색-독립 backstop +
  경로 분류 데이터*만 담는다.
- `pack._phase2`(build/unit_test/verification_stages)는 Phase 2에서 evaluator가 데이터-주도
  스테이지 루프로 소비한다. Phase 1에서는 참조하지 않는다.
