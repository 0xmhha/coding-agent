---
description: 작업 상태 조회. 특정 티켓 또는 전체 활성 작업의 진행 상황을 출력.
argument-hint: "[JIRA-ID] (생략 시 전체 활성 작업)"
---

# /coding-agent:status

작업 상태를 조회한다.

## JIRA-ID 지정 시

- 현재 상태 + 현재 에이전트
- 브랜치명 (IMPLEMENTATION 이후)
- 시작 시각 + 마지막 활동 시각
- 실패 이력 요약 (failure_summary)
- 생성된 아티팩트 목록
- IMPLEMENTATION 단계: step별 진행률 (✓/◐/○)

## JIRA-ID 미지정 시

- 전체 활성 작업(in_progress, BLOCKED) 목록 출력
- 각 작업: ticket_id, 현재 상태, 마지막 활동 시각
