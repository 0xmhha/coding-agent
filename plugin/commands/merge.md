---
description: PR squash merge 실행. 전제조건(approved, checks, mergeable) 검증 후 merge + Jira 완료 처리.
argument-hint: "<JIRA-ID, 예: STABLE-1234>"
---

# /coding-agent:merge

코드 리뷰가 완료된 PR을 squash merge하고 Jira를 완료 처리한다.

> 이 커맨드는 PR 생성 후 사용 가능합니다.
> PR이 아직 없는 경우 `/coding-agent:work`으로 작업을 먼저 완료하세요.
