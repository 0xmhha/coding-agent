---
name: bench-planner-skills
model: claude-opus-4-7
description: |
  Benchmark mode C (code + comprehension skills) planner. Same job and artifacts
  as the real planner, but with NO cks retrieval — it locates code with
  grep/read and interprets it with comprehension skills (path classifier +
  domain invariant backstop). Used by the bench-orchestration skill to measure
  whether a skill-only regime approaches cks quality at lower cost. Never used
  in production /work.
tools:
  - Read
  - Write
  - Edit
  - Bash
  - Grep
  - Glob
skills:
  - state-machine
  - template-parse
  - stablenet-context
  - stablenet-invariants
---

# Bench Planner — Mode C (code + comprehension skills)

A/B/C comparison variant of the planner (see `bench-planner-codeonly.md` for the
three-mode framing). Mode C = grep/read for *finding* code + comprehension
skills for *interpreting* it, but still **no cks retrieval**. The model is fixed
to `claude-opus-4-7` so the comparison isolates the information regime.

## Contract: identical artifacts

Produce the exact same artifacts as `planner.md` so the shared `implementer` and
`evaluator` consume them mode-blind. **Follow `planner.md` exactly for §4
PLANNING and §5 DESIGN** (and §6/§7/§8). Only ANALYSIS differs — replace cks
retrieval with the procedure below.

`analysis.md`, `related-code.json`, `plan.md`, `design-v{N}.md`,
`design-changelog.md` are REQUIRED pipeline state artifacts — `Write` them to
`workspace_dir`. The general agent rule *"do NOT write report/.md files; return
findings as text"* does NOT apply here; returning them only as chat text BREAKS
the pipeline.

## ANALYSIS (code + skills)

### C.0 No backend health check
No cks in this mode. Record in analysis.md: "Retrieval backend: NONE (mode C,
code + comprehension skills) — code located by grep/read, interpreted via the
stablenet-context classifier and the stablenet-invariants backstop; no semantic
or graph retrieval."

### C.1 Load + parse the ticket
Identical to `planner.md` §3.1.

### C.2 Locate relevant code by search
Same as `bench-planner-codeonly` §B.2 (Grep/Glob/Read to find candidate files,
callers, and impact). Persist `related-code.json` with `"mode": "code_skills"`.

### C.3 Interpret with comprehension skills (the mode-C differentiator)
Use the granted skills to interpret what search surfaced — this is what
distinguishes mode C from mode B:

```
classify   = stablenet-context.classify_domain(file_paths, symbols)   # path-based modules
complexity = stablenet-context.estimate_complexity(classify.domains, change_summary)
invariants = stablenet-invariants (always-on backstop): check the change against
             the 5 byzantine-fairness invariants (equal power, epoch asymmetry,
             round-change neutrality, integer quorum, sticky-proposer concentration)
```

Carry the classifier output and any invariant concern into analysis.md. Note
that, unlike mode A, the invariants here come from the static backstop skill, not
from live cks `guidance` fields — so they are general, not change-specific.

### C.4 Produce analysis.md + persist + transition
Same as `planner.md` §3.6–§3.8, with the mode-C caveats recorded.

## Tool & safety policies
Same as `planner.md` §9.
