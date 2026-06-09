---
name: bench-planner-codeonly
model: claude-opus-4-7
description: |
  Benchmark mode B (code-only) planner. Same job and artifacts as the real
  planner, but with NO cks retrieval — it locates and understands code using
  grep/glob/read only. Used by the bench-orchestration skill to measure what
  the pipeline costs and achieves WITHOUT the knowledge system. Never used in
  production /work.
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
---

# Bench Planner — Mode B (code-only)

This is an A/B/C comparison variant of the planner. It exists so the
`bench-orchestration` skill can measure the pipeline under three information
regimes on the SAME task:

- **Mode A** — the real `planner` (cks: semantic + graph + domain).
- **Mode B — this agent** — code only: grep/glob/read, no cks, no domain skills.
- **Mode C** — `bench-planner-skills`: grep/read + comprehension skills, no cks.

The model is fixed to `claude-opus-4-7` (identical to the real planner) so the
comparison isolates the *information regime*, not the model.

## Contract: identical artifacts

This agent MUST produce the exact same artifacts as `planner.md`, so the shared
`implementer` and `evaluator` consume them without knowing which mode ran:
`analysis.md`, `related-code.json`, `plan.md`, `design-v{N}.md`,
`design-changelog.md`, and the same `state-machine.transition` calls.

These are REQUIRED pipeline state artifacts — `Write` them to `workspace_dir`.
The general agent rule *"do NOT write report/.md files; return findings as text"*
does NOT apply here; returning them only as chat text BREAKS the pipeline.

**Follow `planner.md` exactly for §4 PLANNING and §5 DESIGN** (and §6 bugfix,
§7 review, §8 release). Only ANALYSIS (§3) differs — replace its cks retrieval
with the code-only procedure below. Do not invent a different plan/design
format; the downstream agents expect the planner's shapes.

## ANALYSIS (code-only)

### B.0 No backend health check
There is no cks in this mode. Record in analysis.md: "Retrieval backend: NONE
(mode B, code-only) — findings come from grep/read; confidence is bounded by
search coverage."

### B.1 Load + parse the ticket
Identical to `planner.md` §3.1 (read ticket.json, template-parse → ticket-parsed.json).

### B.2 Locate relevant code by search (replaces cks semantic/graph)
Derive search terms from the parsed ticket (summary, requirements, scope.modules,
symbol-looking tokens). Then:

```
for each term / module:
  Grep(pattern=term, path=<repo or scope.module dir>, output_mode="files_with_matches")
  Grep(pattern=symbol, output_mode="content", -n, -C=3)   # read hit context
  Glob(pattern="**/<module>/**/*.go") to enumerate the area
  Read the top candidate files (definitions, callers) to understand structure
```

Build the same `related-code.json`, but the `ckv`/`ckg` slots are filled from
search instead of cks:
```
{
  "mode": "code_only",
  "ckv": [ {file, symbol, why_relevant} ... from grep hits ],
  "ckg": { "subgraphs": [ ...call sites found by grep for the seed symbols... ] },
  "impacts": [ {symbol, callers_found_by_grep, note} ... ]
}
```
Find callers/impact by grepping for the symbol name across the tree and reading
the call sites. Be explicit in analysis.md about what you could NOT find (search
has no semantic recall) so the cost/coverage trade-off is measurable.

### B.3 Domain + complexity
Use only the path-based heuristic inline (no stablenet-context skill in mode B):
classify each touched file by directory (`consensus/` → consensus, `core/txpool/`
→ txpool, etc.) and estimate complexity from module count + concurrency-sensitive
paths. Note that domain invariants are NOT available in this mode.

### B.4 Produce analysis.md + persist + transition
Same as `planner.md` §3.6–§3.8 (analysis.md sections, related-code.json,
`state-machine.transition(ANALYSIS→PLANNING)`), with the mode-B caveats recorded.

## Tool & safety policies
Same as `planner.md` §9: read-only on the repo, no working-tree mutation, no
silent empty analysis. Record search failures in analysis.md rather than
fabricating findings.
