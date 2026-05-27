---
name: state-machine
description: "coding-agent 파이프라인의 상태 전이 관리. state.json CRUD, 전이 조건 검증, failure 로깅, 중단 복구."
type: skill
---

# State Machine

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

## state.json 스키마

```jsonc
{
  "ticket_id": "STABLE-1234",
  "created_at": "ISO timestamp",
  "workspace_dir": ".coding-agent/tickets/STABLE-1234_20260527_000000",
  "ticket_type": "feature",
  "current_state": "TICKET_INTAKE",
  "current_agent": null,
  "states": {
    "TICKET_INTAKE": { "status": "pending" },
    "ANALYSIS": { "status": "pending" },
    "PLANNING": { "status": "pending" },
    "DESIGN": { "status": "pending", "revision": 0 },
    "IMPLEMENTATION": { "status": "pending", "branch": null, "plan_progress": null },
    "EVALUATION": { "status": "pending", "results": {} },
    "COMPLETION": { "status": "pending", "pr_url": null }
  },
  "failure_log": [],
  "failure_summary": { "total_failures": 0, "by_state": {}, "by_type": {}, "recurring_patterns": [] },
  "config": { "max_design_revisions": 3, "max_eval_cycles": 3 }
}
```
