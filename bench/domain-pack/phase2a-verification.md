# P1 Phase 2a — verification record (branch `p1-phase2-domain-pack-wire`, 2026-06-22)

Phase 2a rewired analyzer/planner/evaluator/bench-analyzer-skills from the
go-stablenet-specific skills to the generic `domain-pack` loader. Four verification
layers; **three pass here, the fourth needs a live full-pipeline run** (merge gate).

## ✅ 1. Content preservation (byte-level)
The moved domain content is identical to the original (no loss in the move).
```
diff <(git show b33931f:plugin/skills/stablenet-invariants/SKILL.md | sed -n '/^1\. /,$p') \
     <(sed -n '/^1\. /,$p' plugin/domains/go-stablenet/invariants.md)   → IDENTICAL (11 invariants + footer)
diff <(git show b33931f:plugin/skills/stablenet-context/SKILL.md | sed -n '/file_path contains/p') \
     <(sed -n '/file_path contains/p' plugin/domains/go-stablenet/context.md) → IDENTICAL (11 path rules)
```

## ✅ 2. Deterministic loader resolution
`project_id=go-stablenet` → `domain-pack.json` → referenced files resolve to real,
non-empty content: invariants.md (11 invariants), context.md (11 path rules).
(Reproduce: see `check.py` + the resolution snippet in the Phase-2a session log.)

## ✅ 3. Loader followability — live agent (the §3.1 reliability risk)
A fresh agent given ONLY the `domain-pack` loader skill (no prior go-stablenet
knowledge) was asked to resolve the pack, classify 4 paths, and quote 3 invariants.
Result — followed the loader end-to-end with **no ambiguity/guessing**:
- read exactly: `skills/domain-pack/SKILL.md` → `domains/go-stablenet/domain-pack.json`
  → `invariants.md` + `context.md`;
- classified consensus/wbft/core/core.go→consensus, core/txpool/...→txpool,
  eth/gasprice/anzeon.go→eth/les, miner/worker.go→miner (all correct);
- quoted EQUAL POWER / EPOCH-LENGTH ASYMMETRY / ROUND-CHANGE NEUTRALITY (correct);
- project_id fallback worked as specified.
→ Empirical evidence (not assertion) that the loader+runtime-Read mechanism is
  followable by an agent reading only the pack files.

## 🔴 4. Full-pipeline no-regression (NOT done here — merge gate)
Does the whole analyzer→planner→implementer→evaluator pipeline still produce an
equivalent fix on a real go-stablenet bugfix ticket vs `fcore-baseline`? Needs live
cks + chainbench + the full pipeline + tokens → a capable session. Layers 1-3
de-risk the loader mechanism specifically; layer 4 confirms end-to-end equivalence.

**Merge rule:** do not merge `p1-phase2-domain-pack-wire` into main until layer 4 passes.
