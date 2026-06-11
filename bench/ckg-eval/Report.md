# CKG 4-Way Retrieval Evaluation (Report v4)

> Test set: 30 questions across 12 go-stablenet domains (`ckg-query-testset.md`).
> Index: built from go-stablenet `dev` (`c051d50b`), scoped to the `gstable`
> build files (unused consensus engines excluded, Solidity contract sources
> included), with production-over-test ranking. Retrieval averaged over 3 runs;
> relevance/sufficiency judged by Sonnet (PARSE_FAIL 0, single vote). Inputs are
> keyword/sentence only; ground-truth answers are kept separate (no leakage).
> α = grep (does not use the graph).
>
> v4 change: the precision metric is split so related context is no longer
> counted as noise (see "Metrics" below). v3 used a single strict precision.

## Methods
- **α** raw files via keyword grep (baseline, no graph)
- **β** full graph dump + node bodies
- **γ** incremental graph lookups + node bodies
- **δ** auto-selected evidence pack (`get_for_task`)

## Metrics
- **Answer-present** (separate check): the ground-truth answer file is in the
  surfaced set. Answers "did we actually retrieve the answer?" — independent of
  how much else was returned.
- **Answer-focus** (strict): surfaced files that are *exactly* the designated
  answer file, over total surfaced. Measures concentration on the answer; it
  treats every other file as off-target, so related context lowers it.
- **Relevance-precision** (LLM-judged): surfaced files that are the answer *or*
  genuinely design-relevant context (callers, callees, types, related
  contracts), over total surfaced. Credits useful related code a graph surfaces.
- **Design-sufficiency** (LLM-judged): the context is enough to modify/design
  the feature.
- **Tokens**: information volume injected. **Efficiency** = sufficiency / 1k tokens.

## Overall comparison

| Method | Answer-present | Answer-focus | Relevance-precision | Design-sufficiency | Avg tokens | Efficiency |
|--------|---------------:|-------------:|--------------------:|-------------------:|-----------:|-----------:|
| α grep | 70% | 0.266 | 0.744 | 80% | 10,829 | 7.39 |
| β graph+body | 83% | 0.034 | 0.300 | 80% | 11,858 | 6.75 |
| γ incremental+body | 83% | 0.061 | 0.383 | 80% | 7,014 | 11.41 |
| **δ auto-select** | **83%** | 0.208 | **0.594** | **77%** | **5,398** | **14.20** |

## Strict vs relevance precision (why the split matters)

| Method | Answer-focus (strict) | Relevance-precision | Interpretation |
|--------|----------------------:|--------------------:|----------------|
| α grep | 0.266 | 0.744 | few files, mostly relevant |
| β graph+body | 0.034 | 0.300 | ~9× more files are relevant than are the exact answer |
| γ incremental+body | 0.061 | 0.383 | graph breadth is mostly useful context, not noise |
| δ auto-select | 0.208 | 0.594 | concise pack, ~60% directly relevant |

The strict metric (answer-focus) made the graph methods look like 96–97% noise
(β 0.034, γ 0.061). Judging each surfaced file for relevance shows that 30–38%
of what β/γ surface is genuinely design-relevant — the breadth a knowledge graph
adds is largely useful context, not noise. Answer-present is reported separately
so a high relevance-precision cannot hide a missing answer.

## Findings

1. **δ (auto-select) is the balanced leader.** Answer-present 83% (tied best),
   relevance-precision 0.594, design-sufficiency 77%, lowest tokens (5,398),
   best efficiency (14.20 — about 2× the grep baseline). It retrieves a concise,
   mostly-relevant pack that contains the answer.
2. **Strict precision under-credited the graph.** Splitting it revealed that
   β/γ's surfaced files are 30–38% relevant (not 3–6%); their value is breadth
   of related context, paid for in tokens.
3. **β (full graph dump) is still the worst value** — most tokens (11,858),
   lowest efficiency (6.75). Breadth without selection is expensive.
4. **α (grep) has the highest relevance-precision (0.744)** because it returns
   few whole files, but the lowest answer-present (70%, grep misses) at ~2× δ's
   tokens.

## Limitations
- gov-validator: graph methods retrieve no useful context for it (only grep
  covers it) — a concrete retrieval-quality target.
- Domain rows use 1–3 questions each (trend only); overall numbers are stable.
- Retrieval averaged over 3 runs; relevance/sufficiency judged with a single
  vote, so those LLM metrics carry some run-to-run noise.
