---
name: stablenet-invariants
description: "go-stablenet의 항상-켜진 byzantine-fairness 핵심 불변식(L3 backstop). 검색 품질과 무관하게 합의 안전성·공정성 판단의 기준선을 제공한다. Planner는 설계가 이를 위반하지 않도록, Evaluator는 diff가 이를 깨지 않았는지 판정하는 데 쓴다."
type: skill
---

# StableNet Critical Invariants — pointer (content moved to the domain pack)

> **P1 Phase 1 (2026-06-22):** 이 backstop의 본문은 단일 소스로 이동했다 —
> **`plugin/domains/go-stablenet/invariants.md`**. 내용·동작은 동일하다(무리팩터 이동).

이 스킬을 쓰는 에이전트는 **`plugin/domains/go-stablenet/invariants.md` 를 `Read` 하여**
그 11개 불변식을 항상-켜진 L3 backstop으로 적용한다(설계 시 위반 금지, diff 판정 시 기준).

> Phase 2 예정: 에이전트 frontmatter가 `stablenet-invariants` 대신 제너릭 `domain-pack`
> 로더를 참조하게 되면, 로더가 `state.project_id`로 활성 팩의 `invariants` 파일을 해석한다.
> 그 시점에 이 포인터 스킬은 제거된다. (ADR `docs/domain-pack-contract-adr-2026-06-22.md`)
