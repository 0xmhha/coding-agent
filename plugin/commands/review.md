---
description: PR 코드리뷰 피드백 반영. 리뷰 코멘트 수집 → 구조화 → 수정 작업 사이클 진입.
argument-hint: "<PR URL 또는 #number, 예: #456>"
---

# /coding-agent:review

PR 코드 리뷰 피드백을 읽고 수정 작업을 수행한다.

## 동작

1. **PR 정보 수집**: `gh pr view` + `gh api` 로 리뷰 코멘트 수집
2. **JIRA-ID 추출**: 브랜치명 `feature/{JIRA-ID}` 또는 PR body에서 추출
3. **작업 폴더 탐색**: `.coding-agent/tickets/{JIRA-ID}_*` 최신 폴더
4. **리뷰 피드백 구조화**:
   - 파일별 인라인 코멘트 파싱
   - 유형 분류: bug_fix, security, test_addition, code_quality, architecture, question, nit
   - 심각도: critical, high, medium, low
   - `review-feedback-{N}.md` 생성
5. **ANALYSIS 상태로 재진입** (review cycle)
6. **Orchestrator Agent 디스패치**: 리뷰 기반 재작업

## 에러 처리

- PR URL에서 JIRA-ID 추출 불가 → 유저에게 JIRA-ID 입력 요청
- 작업 폴더 미존재 → 새 작업 폴더 생성 후 진행
- 리뷰 코멘트 없음 → "리뷰 코멘트가 없습니다" 알림
