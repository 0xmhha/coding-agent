---
name: state-machine
description: |
  State transition management for the coding-agent pipeline.
  Manages state.json CRUD, transition validation, failure logging, and recovery.
---

# State Machine Skill

coding-agent 파이프라인의 상태 전이를 관리한다.

## 제공 기능

### init_state(ticket_id, ticket_type, workspace_dir)
state.json을 생성하고 TICKET_INTAKE 상태로 초기화한다.

### get_current_state(workspace_dir)
현재 상태, 에이전트, 진행률을 반환한다.

### transition(workspace_dir, from_state, to_state, artifacts?)
전이 조건을 검증하고, 충족 시 상태를 업데이트한다.
미충족 시 어떤 조건이 실패했는지 명시적으로 반환.

### update_step_progress(workspace_dir, step_id, status, checkpoint?)
IMPLEMENTATION 단계의 step 진행 상태와 checkpoint를 업데이트한다.

### log_failure(workspace_dir, failure_entry)
failure_log에 실패 기록을 추가하고 failure_summary를 자동 업데이트한다.
recurring_patterns도 갱신.

### get_resume_point(workspace_dir)
중단된 작업의 재개 지점을 반환한다: (current_state, last_step, last_checkpoint).

## 전이 조건

| From → To | 조건 |
|-----------|------|
| TICKET_INTAKE → ANALYSIS | ticket.json 존재 + sensitive_check CLEAN |
| ANALYSIS → PLANNING | analysis.md + related-code.json 존재 |
| PLANNING → DESIGN | plan.md 존재 |
| DESIGN → IMPLEMENTATION | design-v{N}.md 존재 + revision ≤ max |
| IMPLEMENTATION → EVALUATION | 모든 step completed + uncommitted 없음 |
| EVALUATION → COMPLETION | 모든 test result PASS |
| EVALUATION → ANALYSIS | 하나 이상 FAIL + cycles < max |
| * → BLOCKED | cycles ≥ max 또는 revisions ≥ max |
