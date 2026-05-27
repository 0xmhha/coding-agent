---
name: status
description: Check work status for a specific ticket or all active work.
arguments:
  - name: jira_id
    description: "Jira ticket ID (optional - shows all active work if omitted)"
    required: false
---

# /status [JIRA-ID]

작업 상태를 조회한다.

## 동작

### JIRA-ID 지정 시
1. `.coding-agent/tickets/{JIRA-ID}_*` 모든 작업 폴더 탐색
2. 가장 최근 폴더의 state.json 로드
3. 출력:
   - 현재 상태 + 현재 에이전트
   - 브랜치명
   - 시작 시각 + 마지막 활동 시각
   - 실패 이력 요약 (failure_summary)
   - 생성된 아티팩트 목록
   - IMPLEMENTATION 단계 시: step별 진행률 (✓/◐/○)

### JIRA-ID 미지정 시
1. `.coding-agent/tickets/` 전체 스캔
2. in_progress 또는 BLOCKED 상태인 작업만 출력
3. 각 작업: ticket_id, 현재 상태, 마지막 활동 시각

## 출력 예시

```
┌─ STABLE-1234 ──────────────────────────────────┐
│ State: IMPLEMENTATION (step 2/5)                │
│ Branch: feature/STABLE-1234                     │
│ Started: 2026-05-27 00:00                       │
│ Last activity: 2026-05-27 01:30                 │
│ Failures: 1 (unit_test)                         │
│                                                  │
│ [Step 1] ✓ Add interface                        │
│ [Step 2] ◐ Implement logic (checkpoint: 70%)    │
│ [Step 3] ○ Add tests                            │
│ [Step 4] ○ Update integration tests             │
│ [Step 5] ○ Update docs                          │
└──────────────────────────────────────────────────┘
```
