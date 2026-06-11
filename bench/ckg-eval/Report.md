# CKG 4-Way Retrieval Evaluation (Report v3)

> Test set: 30 questions across 12 go-stablenet domains (`ckg-query-testset.md`).
> Index: built from go-stablenet `dev` (`c051d50b`), scoped to the `gstable`
> build files (unused consensus engines excluded, Solidity contract sources
> included). Retrieval averaged over 3 runs; LLM relevance/sufficiency judged
> by Sonnet (PARSE_FAIL 0). Inputs are keyword/sentence only; ground-truth
> answers are kept separate (no answer leakage). α = grep (does not use the graph).
>
> v3 change: production code is now ranked above test files by default in the
> auto-selected path (composer test-demotion). v2 measured before that fix.

## Methods
- **α** raw files via keyword grep (baseline, no graph)
- **β** full graph dump + node bodies
- **γ** incremental graph lookups + node bodies
- **δ** auto-selected evidence pack (`get_for_task`)

## Overall comparison

| Method | Relevance | Design-sufficiency | Location hit | Precision | Avg tokens | Test pollution | Efficiency (suf/1k tok) |
|--------|----------:|-------------------:|-------------:|----------:|-----------:|---------------:|------------------------:|
| α grep | 93% | 83% | 70% | 0.266 | 10,829 | 12/90 | 7.66 |
| β graph+body | 93% | 73% | 83% | 0.034 | 11,797 | 63/90 | 6.19 |
| γ incremental+body | 93% | 80% | 83% | 0.061 | 7,016 | 66/90 | 11.40 |
| **δ auto-select** | 93% | **77%** | **83%** | **0.208** | **5,398** | **0/90** | **14.26** |

- Relevance: judge says the retrieved context is on-topic.
- Design-sufficiency: judge says there is enough code to modify/design the feature.
- Location hit: retrieved context includes the ground-truth file (3-run mean).
- Precision: fraction of surfaced files that are on-target.
- Test pollution: cells whose retrieved set contains any `*_test.go` / test file.

## Effect of production-over-test ranking on δ (v2 → v3)

| Metric (δ) | v2 (before) | v3 (after) |
|------------|------------:|-----------:|
| Test pollution | high | **0/90** |
| Location hit | 70% | **83%** |
| Precision | 0.121 | **0.208** |
| Design-sufficiency | 63% | **77%** |
| Efficiency | 11.59 | **14.26** |

Demoting test files lets the implementation file rank into the evidence pack
instead of being crowded out by tests that mention the same symbol more often.

## Domain-level design-sufficiency

| Domain | α | β | γ | δ |
|--------|--:|--:|--:|--:|
| anzeon-gasprice | 100% | 100% | 100% | 100% |
| fee-delegation | 67% | 100% | 67% | 67% |
| gov-council | 100% | 67% | 100% | 100% |
| gov-minter | 100% | 100% | 100% | 100% |
| gov-validator | 100% | 0% | 0% | 0% |
| native-manager | 50% | 50% | 50% | 50% |
| wbft-finalize | 50% | 100% | 100% | 100% |
| wbft-header | 100% | 100% | 100% | 100% |
| wbft-justification | 100% | 100% | 100% | 100% |
| wbft-prepare-commit | 100% | 67% | 67% | 100% |
| wbft-roundchange | 100% | 100% | 100% | 100% |
| wbft-seal | 67% | 67% | 100% | 33% |
| wbft-validator | 67% | 33% | 67% | 67% |

## Findings

1. **δ (auto-select) is the efficiency leader and the cleanest method.** With
   test demotion it reaches 77% design-sufficiency at the lowest token cost
   (5,398) and zero test pollution — efficiency 14.26, roughly 2× the grep
   baseline (7.66). It ties the best location hit (83%) and has by far the best
   precision among graph methods (0.208).
2. **Production-over-test ranking matters.** Before the fix, queries for an
   implementation were crowded out by `*_test.go` files that mention the symbol
   repeatedly. Demoting tests for non-test intents removed that entirely for δ
   and raised its hit rate, precision, and sufficiency.
3. **β (full graph dump) is the worst value even with bodies** — most tokens
   (11,797), lowest efficiency (6.19). Dumping everything is counterproductive.
4. **β/γ still show test pollution** because they call the raw `semantic_search`
   / `get_subgraph` tools directly, which bypass the composer ranking where the
   demotion lives. The production auto-select path (δ) is clean.
5. **α (grep) is a strong but expensive baseline** — high sufficiency (83%,
   whole files) but lowest location hit (70%, grep misses) at ~2× the tokens of δ.

## Limitations
- gov-validator: the graph methods score 0% sufficiency (only grep covers it).
  cks retrieval does not surface the validator-contract initialization usefully —
  a concrete target for retrieval-quality work.
- Domain rows use 1–3 questions each, so per-domain numbers indicate trend only;
  the overall numbers are the stable signal.
- Single judge vote per cell; retrieval averaged over 3 runs. Noise is reduced
  but not eliminated.
