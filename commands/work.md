---
name: work
description: Start work from a Jira ticket. Reads ticket, runs sensitive check, creates workspace, dispatches orchestrator.
arguments:
  - name: jira_id
    description: "Jira ticket ID (e.g., STABLE-1234)"
    required: true
---

# /work <JIRA-ID>

Jira 티켓 기반 자동화 작업을 시작한다.

## 동작

1. **인자 검증**: JIRA-ID 형식 `/^[A-Z]+-\d+$/` 확인
2. **중복 작업 체크**: `.coding-agent/tickets/` 에서 동일 JIRA-ID 활성 작업 탐색
   - BLOCKED 상태 → "이전 작업이 BLOCKED 상태입니다. 재개하시겠습니까?"
   - in_progress → 복구 모드 진입 (마지막 checkpoint에서 재개)
   - completed → 새 작업 폴더 생성
3. **Jira Gateway MCP**로 티켓 읽기 (sensitive filter 자동 적용)
4. **작업 폴더 생성**: `.coding-agent/tickets/{JIRA-ID}_{YYYYMMDD_HHmmss}/`
5. **state.json 초기화** (state-machine skill 사용)
6. **Orchestrator Agent 디스패치**: workspace_dir 전달

## 에러 처리

- JIRA-ID 미입력 → 사용법 안내
- 형식 불일치 → 올바른 형식 예시 제공
- `.coding-agent/` 미존재 → 자동 생성
- Jira 티켓 미존재 → 에러 메시지
- Sensitive check BLOCKED → 민감정보 알림 + 중단
