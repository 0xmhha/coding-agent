---
name: merge
description: Squash merge a PR after all reviewers approve. Updates Jira to Complete.
arguments:
  - name: jira_id
    description: "Jira ticket ID (e.g., STABLE-1234)"
    required: true
---

# /merge <JIRA-ID>

코드 리뷰가 완료된 PR을 squash merge하고 Jira를 완료 처리한다.

## 동작

1. **작업 폴더에서 PR URL 확인**: state.json → COMPLETION.pr_url
2. **전제 조건 검증**:
   - `gh pr view --json reviewDecision` → APPROVED
   - `gh pr checks` → 모든 check PASS
   - `gh pr view --json mergeable` → MERGEABLE
3. **조건 미충족 시**: 어떤 조건이 미충족인지 유저에게 알림, 머지 중단
4. **Squash merge 실행**:
   ```
   gh pr merge {number} --squash --delete-branch \
     --subject "{JIRA-ID}: {summary}" \
     --body "{개별 커밋 목록 + Jira URL + PR #}"
   ```
5. **Jira 업데이트**:
   - jira_add_comment: "Merged. Commit: {hash}"
   - jira_update_status: "Complete"
6. **로컬 정리**:
   - git checkout main && git pull
   - state.json → COMPLETED

## 에러 처리

- PR 미존재 → "이 티켓에 대한 PR이 없습니다"
- merge 충돌 → rebase 시도, 실패 시 유저에게 수동 해결 요청
- Jira 업데이트 실패 → 경고 (merge는 이미 완료)
