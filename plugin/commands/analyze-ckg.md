---
description: ckg(graph MCP) + skill + grep 기반 근본원인 분석 트랙. cks를 거치지 않고 ckg 직접 검색으로 go-stablenet 결함의 원인을 진단한다(분석 전용, 코드 미수정).
argument-hint: "\"<증상/분석 요청 텍스트>\"  [--pr <번호 또는 커밋>]"
---

# /coding-agent:analyze-ckg

**ckg-직접 검증 트랙.** 자유 텍스트 증상을 받아 **ckg MCP(그래프 검색) + comprehension skill + grep**
으로 go-stablenet 결함의 근본원인을 분석한다. cks 컴포저를 거치지 않으며(`cks.context.*` 미사용),
implementer/evaluator 파이프라인에도 들어가지 않는 **분석 전용** 커맨드다.

목적: *ckg가 결함 진단에 충분하고 정확한 코드 정보를 제공하는가?* 를 측정한다.
산출물 `analysis.txt` 를 실제 수정 PR과 대조하여 ckg 정보의 충분성/정확성을 평가한다.

> 자율 진입: 사용자 프롬프트 없이 매 실행이 새 작업이다. 로컬 민감정보는 auto-redact.

---

## 0. 인자 형식

- 기본: `/coding-agent:analyze-ckg "consensus isJustified 가 stale view justification 을 통과시키는 원인"`
- 정답 PR(선택, 봉인): `... --pr 85` — 분석 후 대조용으로만 기록한다. **diff/fix 를 읽지 않는다**(정답 누출 방지).

---

## 1. 인자 검증

```
1.1. 인자 파싱
   - 따옴표 본문 → requirement_text
   - --pr <번호|커밋> → pr_ref (선택)
   - 빈 요구사항 → 사용법 출력 후 중단:
     "사용법: /coding-agent:analyze-ckg \"<증상 텍스트>\" [--pr <번호|커밋>]"
```

---

## 2. ckg 백엔드 / 소스 정합성 확인

```
2.1. ckg 그래프 디렉토리 = ${CKG_GRAPH_DIR} (env). 없으면 중단:
     "CKG_GRAPH_DIR 미설정 — settings.json env 에 ckg 그래프 경로를 설정하세요."
2.2. 인덱싱된 소스 루트 확인 (grep/read 정합성의 핵심):
     bash: python3 -c "import json;print(json.load(open('${CKG_GRAPH_DIR}/manifest.json'))['src_root'])"
       → gostablenet_root
     bash: python3 -c "import json;m=json.load(open('${CKG_GRAPH_DIR}/manifest.json'));print(m.get('src_commit',''))"
       → indexed_commit
   gostablenet_root 가 존재하지 않으면 경고를 기록하되 중단하지 않는다(ckg-only 분석으로 진행).
   ※ grep/read 는 반드시 gostablenet_root(=ckg 가 인덱싱한 그 체크아웃)에서 수행해야
     ckg 가 돌려주는 file:line 과 일치한다. 다른 체크아웃을 읽으면 라인이 어긋난다.
```

---

## 3. 작업 폴더 생성 (항상 새 작업)

```
3.1. bash: git rev-parse --show-toplevel 2>/dev/null || pwd  → repo_root
3.2. bash: date -u +"%Y%m%d_%H%M%S"  → timestamp
3.3. workspace = "{repo_root}/.coding-agent/analysis-ckg/{timestamp}"
3.4. bash: mkdir -p {workspace}
```

---

## 4. 로컬 민감정보 스캔 (auto-redact, 하드스톱 없음)

```
requirement_text 에서 명백한 비밀(`sk-`/`ghp_`/`-----BEGIN`/토큰/패스워드)을 탐지하면
해당 값을 "[REDACTED]" 로 치환하고 카운트한다. 절대 중단하지 않는다.
```

---

## 5. analyze-ckg Agent 디스패치

```
5.1. Agent(
       subagent_type="analyze-ckg",
       description="ckg-direct root-cause analysis ({timestamp})",
       prompt=
         "workspace_dir={workspace}\n"
         "gostablenet_root={gostablenet_root}\n"
         "indexed_commit={indexed_commit}\n"
         "pr_ref={pr_ref or 'none'}\n"
         "requirement_text=<<<\n{redacted requirement_text}\n>>>"
     )
5.2. 완료 후 출력:
   "ckg-직접 근본원인 분석 완료. 산출물:
      - {workspace}/analysis.txt       (근본원인 + ckg 충분성 평가)
      - {workspace}/related-code.json (증거 file:line + ckg 갭)
      - {workspace}/ckg-trace.json    (ckg 호출 감사 로그)
    인덱싱 커밋: {indexed_commit}. 정답 PR: {pr_ref or 'none'} (대조는 수동)."
```

---

## 6. 완료 기준 (체크리스트)

- [ ] 빈 요구사항에 사용법 출력
- [ ] CKG_GRAPH_DIR 미설정 시 명확한 에러
- [ ] manifest 에서 src_root/src_commit 해소 → grep/read 정합성 확보
- [ ] 매 실행 고유 timestamp 작업 폴더
- [ ] 로컬 민감정보 auto-redact (하드스톱 없음)
- [ ] analyze-ckg 에이전트가 analysis.txt + related-code.json + ckg-trace.json 생성
- [ ] --pr 지정 시 diff 미열람(봉인)으로 기록
