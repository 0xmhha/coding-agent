---
name: stablenet-context
description: |
  go-stablenet domain knowledge. Module classification, concurrency patterns,
  and context for the Planner agent to understand the codebase structure.
---

# Stablenet Context Skill

go-stablenet(geth fork) 프로젝트의 도메인 지식을 제공한다.

## 프로젝트 특성

- geth(go-ethereum) fork 기반 블록체인 클라이언트
- consensus가 변경됨 (WBFT)
- native coin이 ETH가 아닌 stablecoin
- system contract (GovStaking, GovConfig, GovNCP, GovRewardeeImp) 지원
- 대규모 Go 코드베이스

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
| cmd | cmd/ | CLI 엔트리포인트 |

## 동시성 패턴

- consensus와 txpool은 goroutine 기반 동작이 핵심
- channel로 블록/트랜잭션 전파
- sync.RWMutex로 stateDB 보호
- context.Context로 취소/타임아웃 전파

## 주의사항

- consensus 변경 시 p2p/core에 연쇄 영향 가능
- system contract 수정 시 genesis 설정과 일관성 확인 필요
- 하드포크 파라미터 변경 시 호환성 검증 필수

## 상세화

이 스킬은 실제 go-stablenet 프로젝트 경로가 전달된 후
코드 탐색을 통해 모듈별 상세 정보가 추가된다.
