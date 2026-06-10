#!/usr/bin/env python3
"""run_judge.py — 평가: 검색된 컨텍스트의 관련성·설계충분성을 LLM이 판정.

요구사항(평가#1): "응답 결과의 내용으로 llm이 얻는 정보가 input(쿼리)에 대한 내용으로
구성됐는지, 그 정보로 기능 수정 등 설계 작업이 가능한지" 판정 + 토큰은 검색단계에서 측정.

robustness(지난 Haiku 실패 교훈): 시스템 프롬프트로 JSON만 강제 + 정규식 폴백 파싱 +
1회 재시도 + 기본 모델 사용. 컨텍스트는 비용 한정 위해 ~20k자(=~5k토큰)로 절단(절단표시).
판정 입력은 '쿼리 + 검색 컨텍스트'뿐 — 정답(expected)은 절대 주지 않음.

Usage: python3 run_judge.py [--model <id>] [--cap-chars 20000]
"""
from __future__ import annotations
import json, os, re, subprocess, argparse, sys

HERE = os.path.dirname(os.path.abspath(__file__))
_SYS = (
    "You evaluate a RETRIEVED CODE CONTEXT for a developer question about the go-stablenet "
    "(Go blockchain) codebase. Judge ONLY from the provided context (not prior knowledge):\n"
    "- relevant: the context is about the question's topic (the right area of code).\n"
    "- sufficient: it contains enough of the actual code (the relevant function/type and its "
    "body) that a developer could modify or design that feature from it.\n"
    "- answer_present: the specific code the question asks about is literally present in the context.\n"
    "Respond with ONLY a JSON object on one line, no prose:\n"
    '{"relevant": true|false, "sufficient": true|false, "answer_present": true|false}'
)

def judge(question, context, model, cap):
    if len(context) > cap:
        context = context[:cap] + "\n...[TRUNCATED]"
    user = f"QUESTION:\n{question}\n\nRETRIEVED CONTEXT:\n{context}"
    cmd = ["claude", "-p", "--output-format", "json", "--append-system-prompt", _SYS]
    if model:
        cmd += ["--model", model]
    for attempt in range(2):
        try:
            r = subprocess.run(cmd, input=user, capture_output=True, text=True, timeout=180)
            outer = json.loads(r.stdout)
            txt = outer.get("result", "") or ""
        except Exception as e:
            txt = ""
        # parse the model's JSON verdict
        try:
            obj = json.loads(txt[txt.find("{"): txt.rfind("}") + 1])
            return {k: bool(obj.get(k)) for k in ("relevant", "sufficient", "answer_present")}, txt[:200]
        except Exception:
            low = txt.lower()
            def grab(key):
                m = re.search(rf'"{key}"\s*:\s*(true|false)', low)
                return (m.group(1) == "true") if m else None
            v = {k: grab(k) for k in ("relevant", "sufficient", "answer_present")}
            if any(x is not None for x in v.values()):
                return {k: bool(x) for k, x in v.items()}, txt[:200]
    return {"relevant": None, "sufficient": None, "answer_present": None}, "PARSE_FAIL"

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--model", default=None)
    ap.add_argument("--cap-chars", type=int, default=20000)
    ap.add_argument("--votes", type=int, default=3, help="judge N times, majority vote (noise reduction)")
    args = ap.parse_args()

    queries = {q["id"]: q for q in json.load(open(os.path.join(HERE, "queries.json")))["queries"]}
    rows = [json.loads(l) for l in open(os.path.join(HERE, "results.jsonl"))]
    out = open(os.path.join(HERE, "judged.jsonl"), "w")
    for i, rec in enumerate(rows):
        q = queries[rec["id"]]
        ctx_path = os.path.join(HERE, "runs", rec["method"], rec["id"] + ".txt")
        context = open(ctx_path, encoding="utf-8", errors="replace").read() if os.path.exists(ctx_path) else ""
        votes = [judge(q["query"], context, args.model, args.cap_chars)[0] for _ in range(args.votes)]
        verdict, tally = {}, {}
        for k in ("relevant", "sufficient", "answer_present"):
            yes = sum(1 for v in votes if v.get(k) is True)
            no = sum(1 for v in votes if v.get(k) is False)
            verdict[k] = (yes >= no) if (yes + no) else None
            tally[k] = f"{yes}/{yes+no}"
        rec = {**rec, "judge": verdict, "votes": tally}
        out.write(json.dumps(rec, ensure_ascii=False) + "\n"); out.flush()
        print(f"[{i+1}/{len(rows)}] {rec['id']} {rec['method']:<6} "
              f"rel={verdict['relevant']}({rec['votes']['relevant']}) "
              f"suf={verdict['sufficient']}({rec['votes']['sufficient']})")
    out.close()
    print("\njudged.jsonl written")

if __name__ == "__main__":
    main()
