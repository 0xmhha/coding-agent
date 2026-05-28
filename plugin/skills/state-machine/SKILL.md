---
name: state-machine
description: "coding-agent 파이프라인의 상태 전이 관리. state.json CRUD, 전이 조건 검증, failure 로깅, 중단 복구."
type: skill
---

# State Machine

coding-agent 파이프라인의 상태 전이를 관리한다. 이 skill은 `Read`, `Write`, `Edit`, `Bash` 도구를 사용하여 state.json 파일을 조작한다.

---

## 1. 데이터 모델

### state.json 스키마

```jsonc
{
  "ticket_id": "STABLE-1234",
  "created_at": "2026-05-28T00:00:00Z",
  "workspace_dir": ".coding-agent/tickets/STABLE-1234_20260528_000000",
  "ticket_type": "feature",
  "pipeline_variant": "full",

  "current_state": "TICKET_INTAKE",
  "current_agent": null,

  "states": {
    "TICKET_INTAKE": { "status": "pending", "started_at": null, "completed_at": null, "artifacts": [], "sensitive_check": null },
    "ANALYSIS":      { "status": "pending", "started_at": null, "completed_at": null, "artifacts": [] },
    "PLANNING":      { "status": "pending", "started_at": null, "completed_at": null, "artifacts": [] },
    "DESIGN":        { "status": "pending", "started_at": null, "completed_at": null, "artifacts": [], "revision": 0 },
    "IMPLEMENTATION":{ "status": "pending", "started_at": null, "completed_at": null, "branch": null, "plan_progress": null, "commits": [] },
    "EVALUATION":    { "status": "pending", "started_at": null, "completed_at": null, "results": { "unit_test": null, "lint": null, "security": null, "chainbench": null }, "report_path": null },
    "COMPLETION":    { "status": "pending", "pr_url": null, "merged_at": null, "merge_commit": null }
  },

  "failure_log": [],
  "failure_summary": { "total_failures": 0, "by_state": {}, "by_type": {}, "recurring_patterns": [] },

  "config": {
    "max_design_revisions": 3,
    "max_eval_cycles": 3,
    "impl_model": "sonnet-4.6",
    "planning_model": "opus-4.7"
  }
}
```

### plan_progress 스키마 (IMPLEMENTATION 상태)

```jsonc
{
  "total_steps": 5,
  "steps": [
    {
      "step_id": 1,
      "description": "...",
      "status": "completed",
      "commits": ["a1b2c3d"],
      "started_at": "...",
      "completed_at": "...",
      "last_checkpoint": null
    }
  ]
}
```

### last_checkpoint 스키마

```jsonc
{
  "at": "2026-05-28T01:45:00Z",
  "reason": "token_limit",
  "work_in_progress": "함수 구현 70%. edge case 처리 남음",
  "uncommitted_files": ["consensus/wbft/finalize.go"]
}
```

### failure_entry 스키마

```jsonc
{
  "id": "fail-001",
  "occurred_at": "2026-05-28T01:23:45Z",
  "state": "EVALUATION",
  "agent": "evaluator",
  "step": "unit_test",
  "attempted_action": {
    "description": "...",
    "command": "go test ./consensus/wbft/...",
    "related_plan_step": "plan.md#step-3",
    "related_design": "design-v2.md#section-2.1",
    "modified_files": ["..."]
  },
  "expected_outcome": "TestFinalize PASS",
  "actual_outcome": {
    "type": "test_failure",
    "summary": "panic: nil pointer",
    "details": "...",
    "exit_code": 2,
    "log_file": "logs/eval-fail-001.log"
  },
  "agent_analysis": {
    "root_cause_hypothesis": "...",
    "confidence": "mid",
    "suggested_fix": "..."
  },
  "resolution": {
    "action": "retry_cycle",
    "transitioned_to": "ANALYSIS",
    "retry_count": 1
  }
}
```

---

## 2. 제공 함수

### 2.1 init_state(ticket_id, ticket_type, workspace_dir, pipeline_variant)

**역할**: state.json을 새로 생성하고 TICKET_INTAKE 상태로 초기화.

**입력**:
- `ticket_id` (string): 예 "STABLE-1234"
- `ticket_type` (string): "feature" | "bugfix" | "code_review" | "release"
- `workspace_dir` (string): 작업 폴더 절대 경로
- `pipeline_variant` (string): "full" | "review_only" | "release"

**절차**:

1. 현재 UTC 시각을 ISO 8601 포맷으로 획득.
   ```bash
   date -u +"%Y-%m-%dT%H:%M:%SZ"
   ```

2. 위의 "데이터 모델" 섹션의 state.json 기본 스키마를 사용하여 객체 구성:
   - `ticket_id`, `workspace_dir`, `ticket_type`, `pipeline_variant`를 입력값으로 채움
   - `created_at`을 현재 시각으로 설정
   - `current_state`는 `"TICKET_INTAKE"`
   - 모든 `states[*].status`는 `"pending"`
   - `failure_log`은 빈 배열, `failure_summary`는 기본값

3. `Write` 도구로 `{workspace_dir}/state.json` 생성.

**출력**: 생성된 state.json의 절대 경로 + 초기 객체.

---

### 2.2 get_current_state(workspace_dir)

**역할**: 현재 상태와 진행률을 조회.

**입력**:
- `workspace_dir` (string): 작업 폴더 경로

**절차**:

1. `Read` 도구로 `{workspace_dir}/state.json` 로드.
2. 다음 정보를 추출하여 반환:
   - `ticket_id`, `current_state`, `current_agent`
   - `states[current_state]`의 전체 객체
   - IMPLEMENTATION 단계인 경우: `plan_progress` 요약 (총 step 수, 완료 step 수, 현재 step의 description)
   - `failure_summary` 요약

**출력**: 위 정보를 담은 객체.

---

### 2.3 transition(workspace_dir, from_state, to_state, artifacts)

**역할**: 전이 조건을 검증하고, 충족 시 상태를 업데이트.

**입력**:
- `workspace_dir` (string)
- `from_state` (string): 현재 상태
- `to_state` (string): 전이 목표 상태
- `artifacts` (array of string, optional): 새로 생성된 아티팩트 파일 경로

**절차**:

1. `Read` 도구로 state.json 로드.

2. **현재 상태 일치 확인**:
   - state.current_state == from_state 인지 확인
   - 불일치 시 → `{ "error": "STATE_MISMATCH", "expected": from_state, "actual": state.current_state }` 반환

3. **전이 조건 검증** (from_state → to_state 별):

   **TICKET_INTAKE → ANALYSIS**:
   - `Bash`: `test -f {workspace_dir}/ticket.json && echo OK` → OK 인지 확인
   - state.states.TICKET_INTAKE.sensitive_check 의 result가 "CLEAN" 또는 "REDACTED" 인지 확인 (BLOCKED은 차단)

   **ANALYSIS → PLANNING**:
   - `Bash`: `test -f {workspace_dir}/analysis.md && test -f {workspace_dir}/related-code.json && echo OK`
   - **아티팩트 완전성 검증 (RI-13)**:
     - analysis.md 내용 길이 > 200 자 (빈 파일 차단)
     - related-code.json을 파싱하여 `results` 배열이 존재하고 비어있지 않은지 확인

   **PLANNING → DESIGN**:
   - `Bash`: `test -f {workspace_dir}/plan.md && echo OK`
   - plan.md에 "## Step" 헤더가 최소 1개 이상 존재 (`grep -c "^## Step" {workspace_dir}/plan.md`)

   **DESIGN → IMPLEMENTATION**:
   - design-v{N}.md 파일이 최소 1개 존재 (`ls {workspace_dir}/design-v*.md`)
   - state.states.DESIGN.revision <= config.max_design_revisions

   **IMPLEMENTATION → EVALUATION**:
   - state.states.IMPLEMENTATION.plan_progress.steps 의 모든 step.status == "completed"
   - 모든 step의 commits 배열이 비어있지 않음
   - `Bash`: `cd {workspace_dir 의 git 레포 루트} && git status --porcelain` → 빈 출력 (uncommitted 없음)

   **EVALUATION → COMPLETION**:
   - state.states.EVALUATION.results 의 모든 값이 "PASS" 또는 "WARN"

   **EVALUATION → ANALYSIS** (fail cycle):
   - state.states.EVALUATION.results 중 하나 이상이 "FAIL"
   - 현재까지의 cycle 카운트 (failure_log에서 transitioned_to=="ANALYSIS"인 항목 수) < config.max_eval_cycles

   **\* → BLOCKED**:
   - max_eval_cycles 초과 OR max_design_revisions 초과
   - 조건 충족 시 즉시 BLOCKED 전이 허용

4. **검증 실패 시**:
   ```jsonc
   {
     "error": "TRANSITION_BLOCKED",
     "missing": ["analysis.md not found", "related-code.json is empty"],
     "from": "ANALYSIS",
     "to": "PLANNING"
   }
   ```

5. **검증 성공 시 state.json 업데이트**:
   - `states[from_state].status = "completed"`
   - `states[from_state].completed_at = now()`
   - `states[from_state].artifacts.push(...artifacts)` (중복 제거)
   - `states[to_state].status = "in_progress"`
   - `states[to_state].started_at = now()`
   - `current_state = to_state`
   - `Write` 도구로 state.json 저장

**출력**: 성공 시 `{ "ok": true, "new_state": to_state }`, 실패 시 위 에러 객체.

---

### 2.4 update_step_progress(workspace_dir, step_id, status, checkpoint, commits)

**역할**: IMPLEMENTATION 단계의 step 진행 상태를 업데이트.

**입력**:
- `workspace_dir` (string)
- `step_id` (integer): plan_progress.steps 의 step_id
- `status` (string): "pending" | "in_progress" | "completed" | "failed"
- `checkpoint` (object, optional): last_checkpoint 스키마. null이면 삭제.
- `commits` (array of string, optional): 추가할 commit hash 목록

**절차**:

1. `Read` 도구로 state.json 로드.

2. `states.IMPLEMENTATION.plan_progress.steps` 에서 step_id 매칭되는 항목 찾기. 미존재 시 에러.

3. 매칭된 step 객체 업데이트:
   - `step.status = status`
   - status 가 "in_progress"이고 step.started_at이 null이면 → started_at = now()
   - status 가 "completed"이면 → completed_at = now(), last_checkpoint = null
   - checkpoint 가 제공되면 → step.last_checkpoint = checkpoint
   - commits 가 제공되면 → step.commits 에 추가 (중복 제거)

4. `states.IMPLEMENTATION.commits` 전체 목록에도 commits 추가.

5. `Write` 도구로 state.json 저장.

**출력**: `{ "ok": true, "step_id": step_id, "new_status": status }`

---

### 2.5 log_failure(workspace_dir, failure_entry)

**역할**: failure_log에 실패 기록을 추가하고 failure_summary를 자동 업데이트.

**입력**:
- `workspace_dir` (string)
- `failure_entry` (object): 위 "데이터 모델"의 failure_entry 스키마

**절차**:

1. `Read` 도구로 state.json 로드.

2. **failure_entry.id 자동 부여** (입력에 없으면):
   - `id = "fail-" + zero_pad(failure_log.length + 1, 3)` (예: "fail-001")

3. **failure_entry.occurred_at 자동 부여** (입력에 없으면): 현재 시각.

4. `failure_log.push(failure_entry)`

5. **failure_summary 업데이트**:
   - `failure_summary.total_failures += 1`
   - `failure_summary.by_state[failure_entry.state] = (existing || 0) + 1`
   - `failure_summary.by_type[failure_entry.actual_outcome.type] = (existing || 0) + 1`

6. **recurring_patterns 갱신**:
   - 패턴 키 생성: `failure_entry.actual_outcome.summary` 에서 핵심 토큰 추출
     - 정규화: 소문자, 숫자/주소/해시 마스킹 (예: "nil pointer in consensus/wbft.go:123" → "nil_pointer:consensus/wbft.go")
   - 기존 patterns 에서 동일 key 검색:
     - 존재 → `pattern.occurrences += 1`, `pattern.failure_ids.push(failure_entry.id)`
     - 미존재 → 동일 key 의 failure 가 failure_log 전체에서 2건 이상이면 새 pattern 추가:
       ```jsonc
       { "pattern": "<key>", "occurrences": <count>, "failure_ids": [...] }
       ```

7. `Write` 도구로 state.json 저장.

**출력**: `{ "ok": true, "failure_id": failure_entry.id }`

---

### 2.6 get_resume_point(workspace_dir)

**역할**: 중단된 작업의 재개 지점을 반환.

**입력**:
- `workspace_dir` (string)

**절차**:

1. `Read` 도구로 state.json 로드.

2. **상태별 재개 지점 결정**:

   **current_state == "IMPLEMENTATION"** (가장 정교한 복구):
   - `plan_progress.steps` 에서 첫 번째 non-completed step 탐색
   - 해당 step의 last_checkpoint 존재 시:
     ```jsonc
     {
       "state": "IMPLEMENTATION",
       "step": { "step_id": N, "description": "...", "status": "in_progress" },
       "checkpoint": { "at": "...", "reason": "...", "work_in_progress": "...", "uncommitted_files": [...] }
     }
     ```
   - checkpoint 없이 in_progress인 step:
     - `Bash`: `cd {repo_root} && git diff --name-only` 로 uncommitted 파일 확인
     - 결과를 checkpoint 형태로 구성하여 반환

   **current_state == "ANALYSIS" / "PLANNING" / "DESIGN"** (RI-02 대응):
   - 해당 단계의 아티팩트 존재 여부 확인:
     - ANALYSIS: analysis.md 또는 related-code.json 부분 존재 시 → "partial" 표시
     - PLANNING: plan.md 부분 존재 시 → "partial"
     - DESIGN: design-v{N}.md 가 있지만 design-changelog.md 가 없는 경우 → "review_pending"
   - 반환:
     ```jsonc
     {
       "state": "<current_state>",
       "sub_status": "fresh" | "partial" | "review_pending",
       "existing_artifacts": [...],
       "recommendation": "처음부터 재실행" | "기존 아티팩트 활용하여 이어서"
     }
     ```

   **current_state == "EVALUATION"**:
   - 마지막 test-report.md 존재 여부 확인
   - 진행 중이던 stage(unit_test/lint/security/chainbench) 식별
   - 반환:
     ```jsonc
     { "state": "EVALUATION", "last_stage_completed": "lint", "next_stage": "security" }
     ```

   **current_state == "TICKET_INTAKE" / "COMPLETION"**:
   - 단순 반환: `{ "state": current_state, "step": null, "checkpoint": null }`

   **current_state == "BLOCKED"**:
   - 반환:
     ```jsonc
     {
       "state": "BLOCKED",
       "block_reason": "...",
       "failure_summary": {...},
       "recommendation": "유저 개입 필요"
     }
     ```

**출력**: 위의 상태별 재개 지점 객체.

---

## 3. 전이 규칙 요약표

| From → To | 핵심 조건 |
|-----------|----------|
| TICKET_INTAKE → ANALYSIS | ticket.json 존재 + sensitive_check CLEAN/REDACTED |
| ANALYSIS → PLANNING | analysis.md (>200자) + related-code.json (results 비어있지 않음) |
| PLANNING → DESIGN | plan.md 존재 + ## Step 헤더 ≥1 |
| DESIGN → IMPLEMENTATION | design-v{N}.md 존재 + revision ≤ max_design_revisions |
| IMPLEMENTATION → EVALUATION | 모든 step completed + commits 존재 + uncommitted 없음 |
| EVALUATION → COMPLETION | 모든 test result PASS/WARN |
| EVALUATION → ANALYSIS | 하나 이상 FAIL + cycles < max_eval_cycles |
| * → BLOCKED | cycles ≥ max_eval_cycles 또는 design revision ≥ max |

---

## 4. 사용 예시

### 새 작업 시작
```
1. init_state("STABLE-1234", "feature", ".coding-agent/tickets/STABLE-1234_20260528_120000", "full")
2. ticket.json 작성
3. transition(workspace_dir, "TICKET_INTAKE", "ANALYSIS", ["ticket.json"])
```

### Implementation 중단 후 복구
```
1. get_resume_point(workspace_dir)
   → { state: "IMPLEMENTATION", step: {step_id: 2, ...}, checkpoint: {...} }
2. checkpoint.work_in_progress 를 Implementer agent 에 전달하여 재개
3. step 완료 시: update_step_progress(workspace_dir, 2, "completed", null, ["e4f5g6h"])
```

### 실패 기록
```
1. test 실패 발생
2. log_failure(workspace_dir, {
     state: "EVALUATION",
     agent: "evaluator",
     step: "unit_test",
     attempted_action: {...},
     actual_outcome: { type: "test_failure", summary: "...", ... },
     agent_analysis: {...},
     resolution: { action: "retry_cycle", transitioned_to: "ANALYSIS", retry_count: 1 }
   })
3. transition(workspace_dir, "EVALUATION", "ANALYSIS")
```
