---
name: stablenet-context
description: "go-stablenet 경로 기반 모듈 분류 + 복잡도 추정 헬퍼. 도메인 지식(불변식·system contract·합의 규칙)은 cks 라이브 검색으로 위임한다."
type: skill
---

# Stablenet Context — pointer (content moved to the domain pack)

> **P1 Phase 1 (2026-06-22):** 경로→모듈 분류 데이터와 `classify_domain`/
> `estimate_complexity` 절차는 단일 소스로 이동했다 — **`plugin/domains/go-stablenet/context.md`**.
> 내용·동작은 동일하다(무리팩터 이동).

이 스킬을 쓰는 에이전트는 **`plugin/domains/go-stablenet/context.md` 를 `Read` 하여**
경로 기반 모듈 분류·복잡도 추정을 수행한다. 도메인 *지식*(불변식·contract 이름·합의 규칙)은
이 파일이 아니라 cks 라이브 + `invariants.md` backstop에서 온다.

> Phase 2 예정: 에이전트가 제너릭 `domain-pack` 로더를 참조하면, 로더가 `state.project_id`로
> 활성 팩의 `context_classifier` 파일을 해석하고 classify/complexity 절차를 제공한다.
> 그 시점에 이 포인터 스킬은 제거된다. (ADR `docs/domain-pack-contract-adr-2026-06-22.md`)
