---
name: stablenet-context
description: "go-stablenet 도메인 지식. 모듈 분류(consensus/core/governance/p2p/...), 동시성 패턴, 복잡도 추정."
type: skill
---

# Stablenet Context

go-stablenet(geth fork) 프로젝트의 도메인 지식을 제공한다.

## 프로젝트 특성

- geth(go-ethereum) fork 기반 블록체인 클라이언트
- consensus가 변경됨 (WBFT)
- native coin이 ETH가 아닌 stablecoin
- system contract (GovStaking, GovConfig, GovNCP, GovRewardeeImp) 지원

## 모듈 분류

| 모듈 | 경로 | 특성 |
|------|------|------|
| consensus | consensus/wbft/ | WBFT 합의, goroutine 다수, 동시성 주의 |
| core | core/ | 블록/트랜잭션 처리, genesis, state 관리 |
| governance | governance-wbft/ | system contract, GovStaking 등 |
| p2p | p2p/ | 피어 통신, 네트워크 레이어 |
| rpc | internal/ethapi/ | JSON-RPC API |
| txpool | core/txpool/ | 트랜잭션 풀 관리 |
| state | core/state/ | 상태 DB, 계정/스토리지 |
| params | params/ | 체인 설정, 하드포크 파라미터 |

## 복잡도 추정

- 1 모듈, 동시성 무관 → `simple`
- 1-2 모듈, 동시성 일부 → `moderate`
- 2+ 모듈 또는 consensus 관련 → `complex`

## 상세화

이 스킬은 go-stablenet 프로젝트 경로가 전달된 후
코드 탐색을 통해 모듈별 상세 정보가 추가된다.
