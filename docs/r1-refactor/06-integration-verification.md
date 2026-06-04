# 06 — R1′ Integration Verification Report

> **Date:** 2026-06-04 · **Method:** read-only audit across the five repos (no code modified).
> **Scope:** Verify, case by case, whether the implemented R1′ system actually delivers the
> integration capabilities it was built for. Derives the gap list and the prioritized next actions.
> **Repos audited:**
> - cks — `code-knowledge-system`
> - ckv — `code-knowledge-vector`
> - ckg — `code-knowledge-graph`
> - chainbench — `chainbench`
> - coding-agent — `coding-agent`
> - go-stablenet (source under test) — `stable-net/go-stablenet-latest`

---

## 0. Executive summary

**The engineering (wiring/features) is essentially complete; the shortfalls are content and
measurement, not architecture.**

- cks uses ckv+ckg in-process and exposes them over a 13-tool MCP surface that an external
  consumer (coding-agent) reaches end-to-end. ✅ (cases 1, 2, 3)
- The feature to build ckv/ckg datasets from go-stablenet code + reference docs exists. ✅ (case 4)
- coding-agent is a loadable Claude Code plugin that covers the full development lifecycle. ✅ (cases 6, 8-pipeline)
- **Three real gaps remain:**
  1. **Domain-knowledge content is empty at runtime** — 23 curated entries, **0 verified**, so the
     delivery path emits nothing; 3 expert dimensions are unauthored (case 8-domain). **Highest leverage.**
  2. **MCP existence/health checking is partial** — registered but mostly assumed-connected (case 5).
  3. **The 3-way performance/cost comparison harness does not exist** — the foundation for continuous,
     measurable self-improvement is unbuilt (case 10). **Largest net-new build.**

Verdict legend: **DONE** (capability present & wired) · **PARTIAL** (present but with a load-bearing gap) ·
**MISSING** (capability absent).

---

## 1. Case-by-case findings

### Case 1 — Can cks sufficiently USE ckv and ckg? — **DONE**

- cks imports ckv `pkg/ckv` and ckg `pkg/store` + `pkg/impact` + `pkg/evidence` + `pkg/concurrency`
  **in-process** (normal Go module requires; no `os/exec` in the client dirs).
- All client methods implemented against real backend APIs, none stubbed:
  - ckv (4): `SemanticSearch`, `Health`, `Freshness`, `Close` — `internal/ckvclient/real.go:74,109,126,141`.
  - ckg (10): `BM25Search→SearchFTS`, `FindSymbol`, `Neighbors→NeighborhoodByQname`,
    `ImpactOfChange→impact.Compute`, `EvidenceForIntent→evidence.BuildPack`, `GetNodePRs`,
    `GetSubgraph→SubgraphByQname`, `ConcurrencyImpact→concurrency.Analyze`, `Health`, `Close`
    — `internal/ckgclient/real.go:200,259,315,362,377,409,530,577`.
- `var _ Client = (*Real)(nil)` compile assertions hold; build + client tests pass.
- **Gap:** none. (Minor: the ckg real adapter's live-SQLite open path is exercised at runtime, not in unit tests.)

### Case 2 — External request (coding-agent) → cks → ckv+ckg, end-to-end? — **DONE (defaults to Smart Dummy)**

- The composer pipeline is sequential and each stage hits the stated backend (`internal/composer/composer.go:157-202`):
  intent → **stage1 = ckv `SemanticSearch`** → **stage2 = ckg `BM25Search`+`FindSymbol` (RRF)** →
  **stage3 = ckg `Neighbors`** → budget → sanitize.
- `cks.context.get_for_task` flows through all stages; the other 12 tools call `d.CKV.*`/`d.CKG.*` directly.
  All 13 tools registered (`internal/mcp/server.go:79-91`).
- **Gap (by design):** `config.Default()` ships empty backend paths (`internal/config/config.go:139-142`),
  so out-of-box every tool runs on the **Smart Dummy** (non-crashing placeholder + "run this skill" instructions)
  and `cks.ops.health` reports `degraded`. Real backends activate **iff** the operator points the config at
  built ckv/ckg datasets with a live embedder.

### Case 3 — Does cks run as an MCP server (+ usage)? — **DONE (usage doc gap)**

- `cmd/cks-mcp/main.go:111` serves stdio via `mark3labs/mcp-go v0.52.0`; builds (CGO required — sqlite-vec).
- 13 `cks.*` tools registered; `internal/mcp/schema_golden_test.go` pins the exact set and passes.
- Degraded mode boots without Ollama/bge-m3 (DegradedDummy / FakeEmbedder{Dim:1024}).
- **Run:** `make build-bins` → `./bin/cks-mcp -config ./policies/cks.yaml.example` (only flag is `-config`;
  stdio-only — exits if `listen.mcp_stdio:false`).
- **Gap:** **no client-registration example anywhere** (no `claude mcp add` / `.mcp.json` snippet);
  README is stale ("Pre-α / not yet wired"); `docs/coding-agent-mcp-mapping.md` says **11 tools** (missing
  `concurrency_impact`, `ops.index`).
  - Working registration would be:
    `claude mcp add cks -- /abs/bin/cks-mcp -config /abs/cks.yaml`.

### Case 4 — Build ckv/ckg datasets from go-stablenet code + docs (feature exists)? — **DONE (feature) / content-gated**

- `internal/mcp/ops_index.go` shells `ckv build/reindex` (`--src --out --embedder=ollama --model-name`) +
  `ckg build --src --out`; config-wired via `IndexConfig` (`cmd/cks-mcp/main.go:165-170`).
- `cmd/cks-domain-sync` derives the **ckv view** (`stablenet.yaml`) + **ckg view** (`policy.yaml`) from
  `docs/domain-knowledge/projects/go-stablenet/entries/*.yaml`.
- ckv `build`/`reindex` and ckg `build --src --out --policy-file` CLIs exist; ckg `--policy-file` consumes
  exactly the `governs[]` shape domain-sync emits.
- **Operator sequence:** `cks-domain-sync` → `ckv build … --model-name=bge-m3` → `ckg build … --policy-file` →
  point cks config at the outputs.
- **Gap:** 0 of 23 domain entries are `verified` → domain-sync emits empty today; `ops.index` does **not**
  thread `--policy-file` (governance edges require a manual `ckg build --policy-file`).

### Case 5 — coding-agent checks cks/jira/chainbench MCP existence? — **PARTIAL**

- All three registered in `plugin/.mcp.json` (jira-gateway, cks, chainbench).
- Only **chainbench** has a pre-flight: the evaluator lists tools and diffs against the expected C1 set
  (`plugin/agents/evaluator.md` §7.0) — but this checks tool *names*, not process liveness.
- **cks:** `cks.ops.health` is defined in the contract (`contract/agent-mcp.schema.json`) but **called nowhere**
  in the plugin. Only a freshness gate exists (staleness, not connectivity).
- **jira-gateway:** no connectivity/health/auth-error branch at all — `jira_read_ticket` is called and only the
  content scan result is handled.
- **Gap:** no orchestrator-level "all three servers reachable" pre-flight; `cks.ops.health` is an unused capability.

### Case 6 — Valid Claude Code plugin? — **DONE**

- `plugin/.claude-plugin/plugin.json` present and valid (name, version 0.1.0, description, author, license,
  `mcpServers`).
- commands/ (4), agents/ (4), skills/ (5), hooks/ (hooks.json + 2 executable scripts) all present; all JSON
  valid; all agent/command/skill frontmatter well-formed.
- **No loading blockers.** Unset env vars only prevent the affected server from starting, not plugin load.

### Case 7 — Harness engineering / autonomous operation? — **PARTIAL**

- The orchestrator self-dispatches planner→implementer→evaluator, auto-handles bug-cycle re-entry and BLOCKED,
  and bounds retries (`max_eval_cycles=3`, `max_design_revisions=3`). `state.json` + `get_resume_point` give
  checkpointed, resumable runs. `--local <ticket.json>` enables non-Jira automated runs.
- On a clean ticket, `/work` runs intake→PR with no human input.
- **Gaps to full automation:**
  - No scheduler/daemon launches `/work` itself; invocation is manual; hooks are **log-only** (do not drive state).
  - The "state machine" is **LLM-executed prose**, not a deterministic engine — replay/determinism is best-effort.
  - Human gates fire on: sanitize-REDACTED, release version tag/push, branch conflict, BLOCKED, destructive git.
  - `/merge` is a separate manual command; the pipeline stops at COMPLETION (no auto-merge by design).

### Case 8 (pipeline) — Do commands/skills cover the full lifecycle? — **DONE (9/9; 2 PARTIAL)**

| Stage | Owner | Verdict |
|---|---|---|
| 1. Requirement/issue analysis | `work.md` intake + `template-parse` → `planner.md` §3.1–3.3 | COVERED |
| 2. Identify actual relevant code (cks) | `planner.md` §3.2/§3.4 (semantic_search, get_subgraph, find_callers, concurrency_impact) | COVERED |
| 3. Analyze required modifications (code-grounded) | `planner.md` §3.5 (impact_analysis) → analysis.md/related-code.json | COVERED |
| 4. Design | `planner.md` §5 (per-step design + self-review revision loop) | COVERED |
| 5. Implementation plan | `planner.md` §4 (atomic steps, dependency DAG, verification plan) | COVERED |
| 6. Ralph-loop implementation | `implementer.md` §4 (per-step build verify, checkpoint/resume, split commits) | COVERED |
| 7. chainbench build + test built binary | `implementer.md` §6.1 (build/bin/gstable handoff) + `evaluator.md` §7 | **PARTIAL** (external MCP; absence → FAIL, no substitute) |
| 8. Test planning | `plan.md` §4.4 Verification Plan + `-race` scope from concurrency_impact | **PARTIAL** (embedded, not a discrete deliverable) |
| 9. Commit(s) + PR | `implementer.md` §4.4 + `orchestrator.md` §4 (PR assemble/sanitize/push/labels/Jira) | COVERED |

### Case 8 (domain depth) — Can cks make the LLM a senior go-stablenet expert? — **SCAFFOLD complete / CONTENT absent**

The machinery is built and correct; the knowledge base is shallow-to-empty and **inert at runtime**.

- **Entries:** 23 YAML files, **all `status: needs_verification` (0 verified)**; `cks-domain-sync` emits only
  verified entries → **executed live: 0 categories, 0 policies**. `glossary.yaml` is empty.
- **Source markers:** go-stablenet has **0** `INVARIANT:`/`CONSENSUS:`/`SECURITY:` markers, so the ckv Tier-2
  extractor (which exists) has nothing to extract.
- **Delivery disconnect:** ckv's Go code never references the domain-knowledge entries; the rich prose reaches
  runtime *only* via `cks-domain-sync` → policy `watch_out`/`governs`, which is empty today. `policy/stablenet.yaml`
  (284 lines) is **hand-authored**, not cks-derived.
- **Per-dimension coverage:**

| Dim | Topic | Verdict |
|---|---|---|
| (a) | Blockchain concepts (general) | COVERED-SHALLOW |
| (b) | Cryptography (esp. go-stablenet: signatures/hashing/fee-delegation) | COVERED-SHALLOW (BLS partial; no signature/hash/randao depth) |
| (c) | Philosophy/design vs Ethereum & Kaia (Klaytn) | **ABSENT** |
| (d) | Go concurrency patterns | **ABSENT** |
| (e) | Distributed-network timing / protocol impact | COVERED-SHALLOW (constants only) |
| (f) | Smart/system-contract analysis | COVERED-SHALLOW (addresses; no per-contract logic) |
| (g) | Consensus algorithm (WBFT/QBFT) detail | COVERED-DEEP *(best area; 8 expert entries — but unverified → inert)* |
| (h) | Policy/governance specifics | COVERED-SHALLOW |

- **Quality note:** the entries that exist are expert-grade (line-numbered `code_anchors`, real `invariants`,
  `pitfalls`, `constants`) — the problem is **breadth + verification**, not authoring quality.

### Case 9 — Logging/monitoring of module I/O for debugging? — **PARTIAL**

- **cks (debugging-grade):** `internal/footprint` emits per-stage JSONL events (intent/stage1..stage5 + per-stage
  timings) correlated by `trace_id`/`run_id`, **no sampling**; wired across all stages in `cmd/cks-mcp/main.go:315-345`.
  `internal/auditlog` is append-only + SHA-256 hash-chained. A cks retrieval is fully reconstructable after the fact.
- **coding-agent (artifact/state-grade):** the trace sink `.coding-agent/tickets/{id}_{ts}/logs/` holds `state.json`
  (status/timestamps/`plan_progress`/`failure_log`), `impl.log`, `eval-*.log`, `test-report.md`, and the planner/design
  artifacts. Hooks log subagent completion + git commits.
- **Gap:** coding-agent does **not** capture verbatim sub-agent prompts/responses — `on-agent-complete.sh` records only
  `subagent=<type>`. Reconstruction is artifact-level, not transcript-level. (The contract's "unify on slog across the
  three Go services" is aspirational; cks uses zap for footprint.)

### Case 10 — 3-way performance/safety/cost comparison + report? — **MISSING**

- **Exists (single-mode only):** `cmd/cks-eval` (retrieval P/R/F1 + budget utilization + latency, cks-on, no LLM),
  ckv `internal/eval/prregress` (plan quality, needs an injected LLM), ckg `cmd/eval-gate` (recall/precision regression gate).
- **Absent — everything the 3-way comparison needs:**
  - a **mode-switching runner** for (A) agent+cks, (B) code-only, (C) code+comprehension-skills — none exists;
    the pipeline is hardwired to the full cks path.
  - **LLM token + cost accounting** — the only "tokens" are retrieval-budget estimates, not input/output token usage or USD.
  - a **final-code correctness oracle** per mode (test/chainbench pass) wired as a comparable metric.
  - a **cross-mode safety metric** and an **A/B/C × {correctness, tokens, performance, safety, cost} report generator**.
- The system contract `00 §9` defines this as a **future agent-driven build→evaluate→debug loop, not built code**, and
  declares the underlying thesis (retrieval beats grep) **UNPROVEN**.

---

## 2. Prioritized next actions

> Effort is rough. "Machine" notes where the work must run (content authoring + heavy index builds need go-stablenet +
> a capable/Ollama machine; tooling/scaffold work runs here). P0 is the single highest-leverage item.

| Priority | Action | Why it matters | Effort | Machine |
|---|---|---|---|---|
| **P0** | **Populate + verify domain knowledge.** Promote the 23 entries to `verified`; author the 3 absent dimensions — (c) ETH/Kaia/WEMIX philosophy & divergence, (d) Go-concurrency patterns, and deepen (b) crypto (signature scheme, hashing, fee-delegation crypto, randao); seed `// INVARIANT:`/`// CONSENSUS:` markers in go-stablenet source; populate `glossary.yaml` (via `cks-glossary-gen`). | Opens the **only** path that delivers domain knowledge to the runtime LLM — without it cases 8-domain stays inert regardless of quality. A single `verified` promotion activates `cks-domain-sync` → ckv/ckg policy views. | High (authoring) | go-stablenet machine (+ domain expert session) |
| **P1** | **Build the 3-way comparison harness (case 10).** Net-new: a mode-switching runner (A/B/C), LLM input/output **token + cost accounting**, a per-mode **correctness oracle** (test/chainbench result), a safety metric, and an A/B/C report generator over {correctness, tokens, performance, safety, cost}. Combine with case-9 observability for a continuous-improvement loop. | The foundation for measurable, compounding efficiency gains — and the only way to settle the UNPROVEN "retrieval beats grep" thesis with data (`00 §9`). | High | here (tooling) → runs need Ollama/go-stablenet |
| **P2** | **MCP existence/health pre-flight (case 5).** Add `cks.ops.health` to the planner intake; add a jira transport/auth error branch in `work.md`; add an orchestrator-level "all three servers reachable" pre-flight before dispatch. | Turns three assumed-connected servers into checked dependencies — fewer silent mid-run failures; cheap, high-robustness. | Low | here |
| **P3** | **Index-pipeline + usage-doc fixes (cases 3, 4).** Thread `--policy-file` into `cks.ops.index` (full mode) so governance edges build automatically; add a `claude mcp add` / `.mcp.json` registration example for cks; refresh README + the mapping doc (11→13 tools). | Removes the manual `ckg build --policy-file` step and the first-run doc gap. | Low–Med | here |
| **P4** | **Transcript-grade observability (case 9).** Capture verbatim sub-agent prompt/response (not just `subagent=<type>`) into the per-ticket trace sink; consider a coding-agent footprint analogous to cks's JSONL. | Required substrate for the P1 token/correctness measurement and for debugging agent decisions. | Med | here |

### Sequencing note
P2/P3/P4 are independent, low-risk, and runnable on this machine now. P1 (harness) depends on P4 (token capture) for its
cost/accuracy metrics and is the natural pairing. P0 (content) is the highest-leverage but runs on the go-stablenet machine
with domain-expert input; it unblocks the *quality* the rest of the system measures.

---

## 3. Fact / Opinion

| Type | Statement | Confidence |
|---|---|---|
| Fact | cks imports ckv+ckg in-process; all 14 client methods are real (not stubs); cks-mcp serves 13 tools over stdio with a passing golden test and a degraded fallback. | None |
| Fact | 23 domain entries are all `needs_verification` (0 verified); `cks-domain-sync` emits 0/0; `glossary.yaml` empty; 0 source markers in go-stablenet. | None |
| Fact | `.mcp.json` registers all 3 servers; only chainbench has a pre-flight (tool-name) check; `cks.ops.health` is defined but called nowhere. | None |
| Fact | No A/B/C mode-switching harness, no LLM token/cost accounting, and no comparison-report generator exist in any repo; `00 §9` frames this as a future agent-driven loop. | None |
| Opinion | The engineering is sufficient for the goal; the binding shortfalls are domain content (P0) and the comparison harness (P1), not architecture. | High |
| Opinion | Verifying the entries is the single highest-leverage action — it activates the whole domain-delivery path. | High |
| Opinion | The 3-way comparison is the largest unbuilt capability and the keystone of a self-improving system; existing evals contribute only retrieval-metric primitives. | High |
| Opinion | Case-5 hardening is cheap and worth doing before any unattended/automated runs. | Mid |
