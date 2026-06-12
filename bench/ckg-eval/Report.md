# CKG 4-Way Retrieval Evaluation (Report v7)

> Test set: 30 questions across 12 go-stablenet domains (`ckg-query-testset.md`).
> Index: built from go-stablenet `dev` (`c051d50b`), scoped to the `gstable`
> build files (unused consensus engines excluded, Solidity contract sources
> included). Retrieval averaged over 3 runs; relevance/sufficiency judged by
> Sonnet over all 3 runs (360 cells, PARSE_FAIL 0, single vote). Inputs are
> keyword/sentence only; ground-truth answers are kept separate (no leakage).
> α = grep (does not use the graph).
>
> v7 change: glossary query expansion is now available at the query layer via
> an opt-in `expand` flag on semantic_search / search_text (the same
> concept→symbol expansion get_for_task runs internally), and the graph
> methods β/γ request it. This closes the case where raw semantic search drifts
> to generic infrastructure on domain queries and never surfaces the answer.
> v6 added the `exclude_tests` filter (graph methods → 0 test pollution); v5
> added glossary expansion to δ; v4 split the precision metric. All retained.

## Methods
- **α** raw files via keyword grep (baseline, no graph)
- **β** full graph dump + node bodies (`exclude_tests`, `expand`)
- **γ** incremental graph lookups + node bodies (`exclude_tests`, `expand`)
- **δ** auto-selected evidence pack (`get_for_task`, glossary-expanded)

## Metrics
- **Answer-present** (separate check): the ground-truth answer file is in the
  surfaced set.
- **Answer-focus** (strict): surfaced files that are *exactly* the answer file,
  over total surfaced — (answer)/total.
- **Relevance-precision** (LLM-judged): surfaced files that are the answer *or*
  genuinely design-relevant context, over total — (answer + related)/total.
- **Design-sufficiency** (LLM-judged): the context is enough to modify/design
  the feature.
- **Tokens**: information volume injected. **Efficiency** = sufficiency / 1k tokens.
- **Test-pollution**: cells (of 90 = 30 questions × 3 runs) whose surfaced set
  contains ≥1 test/test-support file. Mirrors `pkg/testpath.IsTest`.

## Overall comparison

| Method | Answer-present | Answer-focus | Relevance-precision | Design-sufficiency | Avg tokens | Efficiency | Test-pollution |
|--------|---------------:|-------------:|--------------------:|-------------------:|-----------:|-----------:|---------------:|
| α grep | 70% | 0.266 | 0.767 | 79% | 10,829 | 7.29 | 15/90 |
| β graph+body | 83% | 0.051 | 0.395 | 88% | 6,842 | 12.83 | **0/90** |
| γ incremental+body | **93%** | 0.124 | 0.445 | 81% | 4,571 | **17.75** | **0/90** |
| **δ auto-select** | **97%** | 0.263 | **0.635** | 83% | 4,800 | 17.36 | **0/90** |

Answer-focus (strict, answer-only) and relevance-precision (answer + design
context) are shown side by side: a low answer-focus (e.g. γ 0.124) with a much
higher relevance-precision (0.445) means most of what the graph adds is useful
context, not noise.

## v6 → v7 change (query-layer glossary expansion)

| Metric | β | γ | δ |
|--------|:-:|:-:|:-:|
| Answer-present | 80% → **83%** | 83% → **93%** | 97% (=) |
| Relevance-precision | 0.328 → **0.395** | 0.410 → **0.445** | 0.651 → 0.635 |
| Design-sufficiency | 80% → **88%** | 80% → 81% | 87% → 83% |
| Avg tokens | 8,615 → **6,842** | 5,972 → **4,571** | 4,800 (=) |
| Efficiency | 9.29 → **12.83** | 13.77 → **17.75** | 18.06 → 17.36 |
| gov-validator sufficiency | **0% → 100%** | 0% → 50% | 100% (=) |

Root cause that this fixes: on domain queries the Korean natural-language
prompt embeds closer to generic Go infrastructure than to the small
domain-specific implementation, so raw semantic_search (which β/γ call
directly, bypassing the composer) drifted — e.g. the gov-validator genesis
question surfaced core/blockchain.go and the .sol interface but never
systemcontracts/gov_validator.go. Expanding the query with glossary
concept→symbol keywords before the vector search makes the answer
(initializeValidator, CalculateMappingSlot) rank first. The expansion also
*lowers* tokens (β −21%, γ −23%) because it targets the small answer files
instead of dragging in large infrastructure files.

## Findings

1. **Query-layer expansion closes the graph methods' worst gap.** The fix that
   only δ enjoyed (it runs the composer) is now opt-in for direct callers.
   γ answer-present jumps 83%→93% and β/γ recover gov-validator (0%→100%/50%).
2. **It is pure upside.** Relevance-precision up, tokens down, efficiency up
   (β +3.5, γ +4.0). γ is now the efficiency leader (17.75) — 93% answer-present
   at 4,571 tokens, edging δ.
3. **δ (auto-select) is still the most reliable** — answer-present 97%, highest
   relevance-precision (0.635); essentially unchanged because it already
   expanded internally.
4. **β (full graph dump) is still the weakest graph method** on tokens/answer-
   present, though its design-sufficiency is high (88%) — breadth helps the
   judge even when the exact answer file isn't top-ranked.
5. **α (grep) is the floor** — unfiltered (15/90 pollution), most tokens, lowest
   answer-present (70%); its high relevance-precision comes from few whole files.

## Domain-level design-sufficiency (v7)

| Domain | α | β | γ | δ |
|--------|--:|--:|--:|--:|
| anzeon-gasprice | 100% | 100% | 100% | 100% |
| fee-delegation | 67% | 67% | 67% | 67% |
| gov-council | 100% | 100% | 100% | 100% |
| gov-minter | 100% | 100% | 100% | 100% |
| gov-validator | 100% | 100% | 50% | 100% |
| native-manager | 50% | 50% | 0% | 50% |
| wbft-finalize | 50% | 100% | 100% | 100% |
| wbft-header | 100% | 100% | 33% | 100% |
| wbft-justification | 100% | 100% | 100% | 100% |
| wbft-prepare-commit | 100% | 78% | 100% | 89% |
| wbft-roundchange | 67% | 100% | 100% | 100% |
| wbft-seal | 67% | 67% | 100% | 44% |
| wbft-validator | 44% | 100% | 67% | 67% |

## Limitations
- gov-validator is now covered by α/β/δ (100%) and partly by γ (50%); the
  remaining γ/native-manager gaps are where a single expanded hit still does
  not carry enough surrounding context for the judge.
- `expand` and `exclude_tests` are opt-in and backward compatible; the benchmark
  has β/γ opt in to model a design/implementation query.
- Domain rows use 1–3 questions each (trend only). Retrieval averaged over 3
  runs; judged over all 3 runs (90 cells/method), single vote.
