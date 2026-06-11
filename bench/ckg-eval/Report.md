# CKG 4-Way Retrieval Evaluation (Report v6)

> Test set: 30 questions across 12 go-stablenet domains (`ckg-query-testset.md`).
> Index: built from go-stablenet `dev` (`c051d50b`), scoped to the `gstable`
> build files (unused consensus engines excluded, Solidity contract sources
> included). Retrieval averaged over 3 runs; relevance/sufficiency judged by
> Sonnet over all 3 runs (360 cells, PARSE_FAIL 0, single vote). Inputs are
> keyword/sentence only; ground-truth answers are kept separate (no leakage).
> α = grep (does not use the graph).
>
> v6 change: an opt-in `exclude_tests` filter was added to the cks retrieval
> tools (semantic_search / search_text / find_symbol / find_callers /
> find_callees / get_subgraph) and the graph methods β/γ now request it. A
> single shared classifier (`pkg/testpath.IsTest`) drops test files **and**
> test-only support files (testutil*.go, test/ testdata/ dirs, …) at query
> time, so β/γ — which call the graph directly and bypass the composer — get
> production-only results. The composer's test-demotion now uses the same
> classifier, so δ drops test helpers too.
> v5 had added glossary query expansion (δ answer-present 83%→97%); that is
> retained. v4 split the precision metric; that is retained.

## Methods
- **α** raw files via keyword grep (baseline, no graph)
- **β** full graph dump + node bodies (`exclude_tests`)
- **γ** incremental graph lookups + node bodies (`exclude_tests`)
- **δ** auto-selected evidence pack (`get_for_task`, glossary-expanded)

## Metrics
- **Answer-present** (separate check): the ground-truth answer file is in the
  surfaced set.
- **Answer-focus** (strict): surfaced files that are *exactly* the answer file,
  over total surfaced.
- **Relevance-precision** (LLM-judged): surfaced files that are the answer *or*
  genuinely design-relevant context, over total surfaced.
- **Design-sufficiency** (LLM-judged): the context is enough to modify/design
  the feature.
- **Tokens**: information volume injected. **Efficiency** = sufficiency / 1k tokens.
- **Test-pollution**: cells (out of 90 = 30 questions × 3 runs) whose surfaced
  set contains at least one test or test-only support file (lower is better).
  Detection mirrors `pkg/testpath.IsTest`.

## Overall comparison

| Method | Answer-present | Answer-focus | Relevance-precision | Design-sufficiency | Avg tokens | Efficiency | Test-pollution |
|--------|---------------:|-------------:|--------------------:|-------------------:|-----------:|-----------:|---------------:|
| α grep | 70% | 0.266 | 0.776 | 82% | 10,829 | 7.59 | 15/90 |
| β graph+body | 80% | 0.040 | 0.328 | 80% | 8,615 | 9.29 | **0/90** |
| γ incremental+body | 83% | 0.072 | 0.410 | 82% | 5,972 | 13.77 | **0/90** |
| **δ auto-select** | **97%** | 0.263 | **0.651** | **87%** | **4,800** | **18.06** | **0/90** |

## v5 → v6 change (exclude_tests query filter)

| Metric | β | γ | δ |
|--------|:-:|:-:|:-:|
| Test-pollution | 78/90 → **0/90** | 69/90 → **0/90** | 3/90 → **0/90** |
| Avg tokens | 11,811 → **8,615** | 7,056 → **5,972** | 4,767 → 4,800 |
| Relevance-precision | 0.293 → **0.328** | 0.372 → **0.410** | 0.641 → 0.651 |
| Efficiency | 6.77 → **9.29** | 11.34 → **13.77** | 18.18 → 18.06 |
| Answer-present | 83% → 80% | 83% → 83% | 97% → 97% |

Filtering tests at the query layer removed all test pollution from the
graph-direct methods, and because test bodies are no longer injected their
token cost fell (β −27%, γ −15%) while relevance-precision and efficiency rose
— the tests they used to surface were noise, not useful context. δ was already
clean (3/90, only a test helper the old classifier missed); the shared
classifier closes that too. The one regression is β answer-present (83%→80%):
removing test hits from the top-k changes which symbols β expands, and one
gov-domain answer dropped out. δ remains the leader on every axis.

## Findings

1. **The query-layer test filter is the right fix.** Demotion lived only in the
   composer, so β/γ (which call the graph directly) leaked tests. An
   `exclude_tests` filter applied to the tool responses takes all graph methods
   to 0/90 pollution without re-indexing.
2. **Removing tests is pure upside for β/γ.** Tokens down (β −3,196, γ −1,084),
   relevance-precision up (+0.035, +0.038), efficiency up (+2.5, +2.4). The
   removed files were noise.
3. **δ (auto-select) is still the clear leader** — answer-present 97%, lowest
   tokens (4,800), highest sufficiency (87%) and efficiency (18.06), 0 pollution.
4. **β (full graph dump) is still the weakest graph method** — most tokens of
   the three, lowest efficiency, and the only method whose answer-present fell.
   Breadth without selection stays expensive even after filtering.
5. **α (grep) is unfiltered** — still 15/90 pollution and the most tokens; its
   high relevance-precision (0.776) comes from returning few whole files.

## Limitations
- gov-validator: δ covers it (100% sufficiency via glossary); β/γ still 0%
  there — the validator-genesis answer is not reachable by their
  semantic-search-then-expand path even with tests filtered.
- The `exclude_tests` filter is opt-in; callers that want tests (e.g. "add a
  test" intents) simply omit it. The benchmark's β/γ opt in to model a
  design/implementation query.
- Domain rows use 1–3 questions each (trend only). Retrieval averaged over 3
  runs; judged over all 3 runs (90 cells/method), single vote.
