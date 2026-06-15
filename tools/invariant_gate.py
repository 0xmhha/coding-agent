#!/usr/bin/env python3
"""
invariant_gate — deterministic invariant→test gate for the evaluator (#4).

Ties together:
  - constraint-assembler index (#3): module → applicable invariants
    (coding-agent/plugin/skills/constraint-assembler/applicable-invariants.yaml)
  - invariant→test catalog (#2): invariant → chainbench test + binding grade
    (chainbench/catalog/invariant-tests.yaml)
  - chainbench JSON results: state/results/*.json (machine-consumable, Gate-0 verified)

Verdict (machine signal, NOT LLM judgment — D-020):
  - BLOCK if any bound invariant test FAILED.
  - BLOCK if a bound invariant has no passing evidence (strict) — unless --check-only.
  - PASS otherwise; surface partial-binding warnings (mechanism verified, distinguishing claim NOT).

Usage:
  invariant_gate.py --module consensus            # applicable invariants for a module
  invariant_gate.py --invariants id1,id2          # explicit set
  invariant_gate.py --module consensus --run      # actually run missing tests via chainbench
  invariant_gate.py --module consensus --json     # machine-readable verdict
Exit code: 0 = gate PASS, 2 = BLOCK, 3 = config/usage error.
"""
import argparse, glob, json, os, subprocess, sys

ROOT = "/Users/kevin/work/github/0xmhha/auto-coding"
DEF_INDEX = f"{ROOT}/coding-agent/plugin/skills/constraint-assembler/applicable-invariants.yaml"
DEF_CATALOG = f"{ROOT}/chainbench/catalog/invariant-tests.yaml"
DEF_CHAINBENCH = f"{ROOT}/chainbench"


def load_yaml(path):
    import yaml
    with open(path) as f:
        return yaml.safe_load(f)


def applicable_invariants(index, module):
    """module invariants (non-doc) + cross_cutting + ALL tier:L3 (always-on backstop)."""
    out = {}
    mods = index["modules"]
    if module and module in mods:
        for i in mods[module]["invariants"]:
            if i.get("kind") != "doc":
                out[i["id"]] = i
    for mm in mods.values():
        for i in mm["invariants"]:
            if i.get("tier") == "L3" and i.get("kind") != "doc":
                out[i["id"]] = i
    for i in index.get("cross_cutting", []):
        out[i["id"]] = i
    return out


def catalog_index(cat):
    """invariant id -> {ref: grade} from the catalog."""
    m = {}
    for inv in cat.get("invariants", []):
        m[inv["id"]] = [(t["ref"], t.get("binding", "unknown")) for t in inv.get("tests", [])]
    return m


def latest_result(chainbench_dir, ref):
    """latest state/results/*.json whose 'test' == ref. Returns (status, file) or (None, None)."""
    pat = os.path.join(chainbench_dir, "state", "results", "*.json")
    best = None
    for fp in glob.glob(pat):
        try:
            d = json.load(open(fp))
        except Exception:
            continue
        if d.get("test") == ref:
            mt = os.path.getmtime(fp)
            if best is None or mt > best[0]:
                best = (mt, d.get("status"), fp)
    return (best[1], best[2]) if best else (None, None)


def run_test(chainbench_dir, ref):
    cmd = [os.path.join(chainbench_dir, "chainbench.sh"), "test", "run", ref]
    subprocess.run(cmd, cwd=chainbench_dir, capture_output=True, text=True, timeout=300)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--module")
    ap.add_argument("--invariants", help="comma-separated explicit invariant ids")
    ap.add_argument("--index", default=DEF_INDEX)
    ap.add_argument("--catalog", default=DEF_CATALOG)
    ap.add_argument("--chainbench-dir", default=DEF_CHAINBENCH)
    ap.add_argument("--run", action="store_true", help="run bound tests that have no result yet")
    ap.add_argument("--check-only", action="store_true",
                    help="do NOT block on not-run tests (only block on explicit FAIL)")
    ap.add_argument("--json", action="store_true")
    args = ap.parse_args()

    try:
        index = load_yaml(args.index)
        cat = catalog_index(load_yaml(args.catalog))
    except Exception as e:
        print(f"[gate] config error: {e}", file=sys.stderr); return 3

    if args.invariants:
        ids = [s.strip() for s in args.invariants.split(",") if s.strip()]
        applicable = {i: {"id": i} for i in ids}
    elif args.module:
        applicable = applicable_invariants(index, args.module)
    else:
        print("[gate] need --module or --invariants", file=sys.stderr); return 3

    rows, blocked, warnings = [], [], []
    for inv_id in sorted(applicable):
        bindings = cat.get(inv_id, [])
        if not bindings:
            rows.append({"id": inv_id, "status": "unbound", "detail": "no chainbench binding — defer to review/perspective"})
            continue
        for ref, grade in bindings:
            status, fp = latest_result(args.chainbench_dir, ref)
            if status is None and args.run:
                run_test(args.chainbench_dir, ref)
                status, fp = latest_result(args.chainbench_dir, ref)
            row = {"id": inv_id, "ref": ref, "binding": grade, "result": status or "not-run", "file": fp}
            if status == "passed":
                if grade == "partial":
                    warnings.append(f"{inv_id} ({ref}): PASS but partial binding — distinguishing claim NOT verified")
            elif status == "failed":
                blocked.append(f"{inv_id} ({ref}): FAILED")
            else:  # not-run
                if not args.check_only:
                    blocked.append(f"{inv_id} ({ref}): no passing evidence (not-run)")
            rows.append(row)

    verdict = "BLOCK" if blocked else "PASS"
    if args.json:
        print(json.dumps({"verdict": verdict, "rows": rows, "blocked": blocked, "warnings": warnings},
                         ensure_ascii=False, indent=1))
    else:
        print(f"=== invariant gate: {verdict} ===")
        for r in rows:
            tag = r.get("result", r.get("status"))
            print(f"  [{tag:>8}] {r['id']}" + (f"  ← {r['ref']} ({r['binding']})" if 'ref' in r else f"  ({r.get('detail','')})"))
        if warnings:
            print("\n⚠ partial-binding (mechanism only, distinguishing claim unverified):")
            for w in warnings: print(f"  - {w}")
        if blocked:
            print("\n🔴 BLOCK reasons:")
            for b in blocked: print(f"  - {b}")
    return 0 if verdict == "PASS" else 2


if __name__ == "__main__":
    sys.exit(main())
