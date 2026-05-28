---
name: stablenet-context
description: "go-stablenet 도메인 지식. geth fork 기반 모듈 분류, 동시성 패턴, 복잡도 추정, 작업 영향 평가."
type: skill
---

# Stablenet Context

go-stablenet(geth fork) 프로젝트의 도메인 지식과 추론 규칙을 제공한다.
Planner Agent의 ANALYSIS 단계에서 사용된다.

> **상세화 시점**: 이 skill의 모듈별 정보는 geth fork의 일반 패턴 + go-stablenet의 알려진 특수성을 기반으로 작성되었다.
> 실제 go-stablenet 프로젝트 경로가 전달되면, Planner가 코드 탐색을 통해 모듈별 정보를 동적으로 보강한다.

---

## 1. 프로젝트 특성

| 항목 | 내용 |
|------|------|
| Base | go-ethereum (geth) fork |
| Consensus | WBFT (BFT 변형) |
| Native coin | Stablecoin (ETH 대체) |
| System contracts | GovStaking, GovConfig, GovNCP, GovRewardeeImp |
| Language | Go |
| 규모 | 대규모 (수천 파일) |

---

## 2. 모듈 분류 표

### 2.1 핵심 모듈

| 모듈 | 경로 패턴 | 역할 | 동시성 |
|------|----------|------|--------|
| **consensus** | `consensus/`, `consensus/wbft/` | WBFT 합의 엔진, 블록 검증/Finalize | **High** — goroutine 다수, 합의 라운드별 채널 |
| **core** | `core/`, `core/types/`, `core/state/`, `core/vm/` | 블록/트랜잭션 처리, EVM, state DB | High — stateDB 동시 접근, txpool 연동 |
| **governance** | `governance-wbft/`, `governance/` | system contract 바인딩 (GovStaking 등) | Medium — staking 변경 시 consensus 통지 |
| **txpool** | `core/txpool/` | 트랜잭션 풀, 멤풀 관리 | High — 다중 producer/consumer goroutine |
| **p2p** | `p2p/`, `p2p/discover/`, `p2p/enode/` | 피어 통신, 디스커버리, devp2p | High — 피어별 goroutine |
| **rpc** | `rpc/`, `internal/ethapi/` | JSON-RPC API | Medium — 요청별 handler |
| **state** | `core/state/`, `trie/` | Merkle Trie, 계정/스토리지 | High — RWMutex 보호 |
| **params** | `params/` | 체인 설정, 하드포크 파라미터 | Low — 대부분 상수 |
| **cmd** | `cmd/`, `cmd/geth/` 또는 `cmd/gstable/` | CLI 진입점 | Low — 초기화만 |
| **eth** | `eth/`, `eth/protocols/` | 이더리움 프로토콜 핸들러 | High — protocol별 goroutine |
| **les** | `les/` | Light Ethereum Subprotocol | Medium |
| **miner** | `miner/` | 블록 생성/제안 | High — sealer goroutine |

### 2.2 도메인 식별 규칙

**파일 경로 기반**:
```
file_path contains "consensus/" → consensus
file_path contains "governance-wbft/" or "governance/" → governance
file_path contains "core/txpool/" → txpool
file_path contains "core/state/" or "trie/" → state
file_path contains "core/" (and not above) → core
file_path contains "p2p/" → p2p
file_path contains "rpc/" or "internal/ethapi/" → rpc
file_path contains "miner/" → miner
file_path contains "params/" → params
file_path contains "cmd/" → cmd
file_path contains "eth/" or "les/" → eth/les
```

**심볼명 기반 (보조)**:
```
"WBFT", "Finalize", "Engine", "Snapshot" + consensus 경로 → consensus
"GovStaking", "GovConfig", "GovNCP", "GovRewardee" → governance
"TxPool", "txList", "txLookup" → txpool
"StateDB", "StateTrie", "stateObject" → state
"Discover", "Server", "Peer" + p2p 경로 → p2p
"Worker", "Sealer", "commitNewWork" → miner
```

---

## 3. 동시성 패턴

### 3.1 주요 동시성 메커니즘

| 패턴 | 사용처 | 주의사항 |
|------|--------|---------|
| **goroutine + channel** | consensus 라운드, p2p 피어, txpool 처리 | 채널 close 후 send 패닉 주의 |
| **sync.RWMutex** | stateDB, txpool, miner worker | Lock/Unlock 짝 + defer 권장 |
| **sync.Map** | txLookup, peer map | iterator는 snapshot 아님 |
| **context.Context** | RPC handler, p2p req/res | 항상 첫 인자, cancel 전파 |
| **sync.WaitGroup** | sealer, downloader | Add는 Wait 시작 전에 |
| **atomic** | shutdown flag, counter | 32bit 정렬 주의 |
| **event.Feed / event.Subscription** | core events, miner events | Unsubscribe 누락 시 leak |

### 3.2 알려진 race condition 핫스팟

- consensus/wbft: 합의 라운드 전환 시 메시지 채널 close 타이밍
- txpool: pending → queued 이동 시 lock 범위
- core/state: snapshot vs commit 동시 접근
- miner: sealer 중단 + 새 작업 시작 race

---

## 4. 작업 영향 평가

### 4.1 복잡도 추정 규칙

```
복잡도 = simple | moderate | complex

규칙:
  1. scope.modules 가 1개 + 동시성 무관 → simple
  2. scope.modules 가 1-2개 + 동시성 일부 관련 → moderate
  3. 다음 중 하나라도 해당 → complex:
     - scope.modules >= 3
     - consensus, txpool, state, miner 중 하나 이상 포함
     - genesis/hardfork 파라미터 변경
     - system contract 변경
     - p2p 프로토콜 변경
     - cross-module 의존 (예: consensus + governance + state)
```

### 4.2 모듈 간 영향 그래프

```
변경 모듈 → 영향 받는 모듈 (전형적 패턴):

consensus 변경 →
  - core (Finalize 호출 경로)
  - miner (블록 생성 시 consensus 호출)
  - p2p (블록 전파)
  - 테스트: consensus/*_test.go, core/blockchain_test.go

governance (system contract) 변경 →
  - consensus (Finalize에서 system contract 호출)
  - core/state (storage layout 변경)
  - genesis (초기 상태 설정)
  - 테스트: governance/*, integration tests

core/state 변경 →
  - 거의 모든 모듈 (Apply, GetState 등 광범위 사용)
  - 특히 consensus, miner, txpool, rpc
  - high risk: backward compatibility

txpool 변경 →
  - miner (pending tx 조회)
  - rpc (eth_sendTransaction 등)
  - core (블록 import 시 tx 제거)

p2p 변경 →
  - eth 프로토콜 (sync, block propagation)
  - les
  - downloader

params 변경 →
  - 하드포크 활성화 시 거의 모든 모듈
  - genesis 호환성 깨질 수 있음
```

### 4.3 권장 테스트 범위

작업 모듈별로 다음 테스트를 권장:

| 작업 모듈 | unit test | integration | ChainBench |
|----------|-----------|-------------|------------|
| consensus | consensus/*_test.go | core/blockchain_test.go | ✅ 필수 |
| governance | governance/*_test.go | + genesis integration | ✅ 필수 |
| core/state | core/state/*_test.go | + core/blockchain_test.go | ✅ 권장 |
| txpool | core/txpool/*_test.go | + core/tx_pool_test.go | 선택 |
| p2p | p2p/*_test.go | + eth/protocols 통합 | 선택 |
| rpc | internal/ethapi/*_test.go | + rpc_test.go | 선택 |
| miner | miner/*_test.go | + core/blockchain | 권장 |
| params | params/*_test.go | + hardfork integration | ✅ 필수 |

---

## 5. 작업 유형별 주의사항

### 5.1 새 기능 추가 (feature)

체크리스트:
- [ ] 기존 인터페이스 변경 여부 (backward compatibility)
- [ ] 새 system contract 추가 시 → genesis config 업데이트 필요
- [ ] consensus rule 변경 시 → 하드포크 파라미터 필요?
- [ ] RPC 메서드 추가 → 권한/rate limit 확인

### 5.2 버그 수정 (bugfix)

체크리스트:
- [ ] regression test 추가 (동일 버그 재발 방지)
- [ ] 영향 범위 (CKG impact 분석)
- [ ] hot path 코드 수정 시 성능 영향 측정
- [ ] race condition 수정 시 → `go test -race` 필수

### 5.3 릴리즈

체크리스트:
- [ ] 모든 unit test 통과
- [ ] ChainBench 통합 테스트 통과 (필수)
- [ ] CHANGELOG.md 업데이트
- [ ] 하드포크 변경 시 호환성 분석
- [ ] 의존성 변경 시 보안 audit
- [ ] git tag + push

---

## 6. 제공 함수

### 6.1 classify_domain(file_paths, symbols)

**입력**:
- `file_paths` (array): 영향 파일 경로 목록
- `symbols` (array, optional): 영향 심볼 목록

**절차**:
1. 각 file_path를 §2.2 규칙으로 분류
2. symbols가 있으면 보조 분류
3. 중복 제거, 빈도순 정렬

**출력**:
```jsonc
{
  "primary_domain": "consensus",
  "domains": ["consensus", "core", "governance"],
  "confidence": "high" | "medium" | "low"
}
```

### 6.2 estimate_complexity(domains, change_summary)

**입력**:
- `domains` (array): classify_domain의 결과
- `change_summary` (string): 변경 내용 요약 (Jira 티켓에서)

**절차**:
1. §4.1 규칙 적용
2. change_summary에 "genesis", "hardfork", "system contract" 키워드 검사
3. 동시성 키워드 ("goroutine", "race", "mutex", "concurrent") 검사

**출력**:
```jsonc
{
  "complexity": "simple" | "moderate" | "complex",
  "reasoning": "..."
}
```

### 6.3 derive_affected_modules(primary_domain, change_type)

**입력**:
- `primary_domain` (string): 주 변경 모듈
- `change_type` (string): "signature" | "logic" | "delete" | "add"

**절차**:
§4.2 영향 그래프를 기반으로 영향 받는 모듈 목록 반환.

**출력**:
```jsonc
{
  "affected_modules": ["core", "miner", "p2p"],
  "required_tests": [...],
  "chainbench_required": true | false
}
```

### 6.4 generate_acceptance_checklist(work_type, primary_domain)

**입력**:
- `work_type` (string): "feature" | "bugfix" | "release"
- `primary_domain` (string)

**절차**:
§5의 작업 유형별 체크리스트 + 도메인별 추가 항목.

**출력**:
```jsonc
{
  "checklist": [
    { "text": "consensus rule 변경 시 하드포크 파라미터 검토", "category": "domain" },
    { "text": "regression test 추가", "category": "work_type" },
    ...
  ]
}
```

---

## 7. 동적 보강 (Planner 사용 시)

go-stablenet 실제 프로젝트 경로(`STABLENET_PROJECT_ROOT` 환경변수)가 전달되면:

1. `Bash`: `find {root} -maxdepth 2 -type d` 로 실제 모듈 구조 확인
2. 위 §2.1 표와 비교, 차이가 있으면 보정
3. `git log --since=90.days -- {module}` 로 최근 활동 분석
4. 모듈별 README/CONTRIBUTING 존재 시 추가 정보 추출

이 보강은 Planner의 ANALYSIS 단계에서 수행되며, 결과는 `analysis.md`의 "도메인 분석" 섹션에 포함된다.
