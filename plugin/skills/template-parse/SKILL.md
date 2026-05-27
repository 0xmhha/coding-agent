---
name: template-parse
description: "Jira 티켓 템플릿 파싱. 작업 유형(Feature/Bug Fix/Code Review/Release) 식별 + 필드 구조화 + 파이프라인 분기 결정."
type: skill
---

# Template Parse

Jira 티켓의 템플릿 유형을 식별하고 필드를 구조화한다.

## 지원 유형

### Feature
필드: 요약, 배경, 요구사항[], 영향 범위(모듈, 파일), 수용 기준[], 참고 자료

### Bug Fix
필드: 요약, 재현 방법[], 기대 동작, 실제 동작, 영향 범위(모듈, 심각도), 수용 기준[]

### Code Review
필드: 요약, 리뷰 대상(파일/모듈, 관점), 리뷰 기준[]

### Release
필드: 버전, 포함 변경사항[], 릴리즈 체크리스트[]

## 파이프라인 분기

- **Feature / Bug Fix** → `pipeline_variant: "full"` (전체 6단계)
- **Code Review** → `pipeline_variant: "review_only"` (구현 생략)
- **Release** → `pipeline_variant: "release"` (태그 + 릴리즈)

## 출력 예시

```json
{
  "work_type": "feature",
  "summary": "...",
  "requirements": ["..."],
  "scope": { "modules": ["consensus", "governance"], "files": [] },
  "acceptance_criteria": ["..."],
  "pipeline_variant": "full",
  "missing_fields": []
}
```
