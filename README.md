# coding-agent

A Claude Code plugin that turns a Jira ticket into a reviewed pull request — autonomously.

`coding-agent` is a multi-agent development pipeline for **go-stablenet** (a geth fork with WBFT consensus). You point it at a ticket; it analyzes, plans, designs, implements, tests, opens a PR, folds in review feedback, and merges — pausing for your confirmation on anything irreversible.

It is built on two ideas:

- **Orchestration over a document-backed state machine.** Every stage writes its artifact (`analysis.md`, `plan.md`, `design-v{N}.md`, `test-report.md`) to disk, so a truncated context or a new session resumes exactly where it left off.
- **Retrieval-grounded decisions.** Instead of guessing about a large unfamiliar codebase, the planner queries a knowledge service (**cks**) with RAG (semantic search) and graph-RAG (call graphs, impact and concurrency analysis), and grounds every design choice in real code.

---

## How it works

```
Jira ticket (STABLE-xxxx)
   │  jira-gateway MCP  ── sensitive-info filter (secrets blocked before they reach the LLM)
   ▼
TICKET_INTAKE → ANALYSIS → PLANNING → DESIGN → IMPLEMENTATION → EVALUATION → COMPLETION
                   │                                                │
              cks retrieval                                   4-stage gate
            (RAG + graph-RAG)                       (unit+race · lint · security · chainbench)
                                                                    │
                                                   PASS → PR + Jira update
                                                   FAIL → bugfix cycle (≤3) or BLOCKED
```

Four isolated agents do the work; the **orchestrator** is the only one that sees the whole flow:

| Agent | Role |
|-------|------|
| **orchestrator** | Drives state transitions, MCP pre-flight, PR/Jira completion, bug-cycle re-entry |
| **planner** | ANALYSIS / PLANNING / DESIGN. The sole cks consumer — RAG + graph-RAG retrieval |
| **implementer** | Branch isolation, one commit per atomic step, build handoff |
| **evaluator** | 4-stage verification: unit (+`-race`), lint/format, security scan, chainbench integration |

It talks to three MCP servers: **jira-gateway** (in this repo, a sensitive-info proxy in front of Jira), **cks** (`code-knowledge-system`, a sibling repo that composes semantic + graph retrieval), and **chainbench** (a sibling repo, the deterministic test runner). The agent-facing tool surface is frozen in `contract/agent-mcp.schema.json` and enforced by `contract/lint-tool-names.sh`.

**Security model.** Sensitive data is blocked *before it reaches the model*, not after. All inbound Jira content passes through the jira-gateway filter (regex + entropy + allowlist → `REDACTED`/`BLOCKED`); all outbound text (PR bodies, commit bodies, Jira comments) passes through the `pr-sanitize` skill using the same `shared/patterns.json`.

---

## Install

The plugin is distributed as a Claude Code marketplace plugin from this GitHub repo.

```
/plugin marketplace add 0xmhha/coding-agent
/plugin install coding-agent@coding-agent
```

Restart Claude Code, then run `/help` — you should see `/coding-agent:work`, `/coding-agent:review`, `/coding-agent:status`, and `/coding-agent:merge`.

> **Local development install.** To run from a clone instead, point your user config at the plugin directory:
> ```jsonc
> { "plugins": { "coding-agent": { "path": "/abs/path/to/coding-agent/plugin" } } }
> ```

---

## Configure

The plugin reads secrets and server locations from environment variables that `plugin/.mcp.json` forwards into the MCP servers. Export them in your shell profile so Claude Code's child processes inherit them.

```bash
# Jira (required) — token: https://id.atlassian.com/manage-profile/security/api-tokens
export JIRA_BASE_URL="https://your-domain.atlassian.net"
export JIRA_USER_EMAIL="you@example.com"
export JIRA_API_TOKEN="atlassian_api_token_here"

# cks — the code-knowledge service (sibling repo; see SETUP.md to build)
export CKS_MCP_BIN="$HOME/Work/code-knowledge-system/bin/cks-mcp"
export CKS_CONFIG="$HOME/Work/code-knowledge-system/cks.yaml"

# chainbench — the deterministic test runner (sibling repo)
export CHAINBENCH_DIR="$HOME/Work/chainbench"
```

| Requirement | Why |
|-------------|-----|
| Claude Code | Hosts the plugin |
| Atlassian (Jira) Cloud | Source of tickets |
| `gh` CLI ≥ 2.50 | PR create / comment / merge |
| `code-knowledge-system` (cks) + Ollama + `bge-m3` | Code retrieval (RAG + graph-RAG). Without it, cks runs **degraded** and the pipeline still works at lower retrieval quality |
| `chainbench` | Evaluator Stage 4 (integration). Skippable; Stage 4 fails loudly if absent |

`code-knowledge-system` and `chainbench` are sibling repositories resolved by path at runtime. Building and indexing them (the slow `bge-m3` embed of go-stablenet) is covered step by step in **[docs/SETUP.md](docs/SETUP.md)**.

After install, verify the contract is intact:

```bash
bash contract/lint-tool-names.sh    # exits 0 when prompt tool names match the schema
```

---

## Usage

| Command | What it does |
|---------|--------------|
| `/coding-agent:work STABLE-1234` | Main entry point. Reads the ticket, runs the full pipeline to a PR. `--local <ticket.json>` runs without Jira |
| `/coding-agent:status [STABLE-1234]` | Progress of one ticket, or all active workspaces |
| `/coding-agent:review <PR-URL>` | Collect PR comments → classify → re-enter the pipeline in bugfix mode |
| `/coding-agent:merge STABLE-1234` | The only command that touches `main`: squash-merge (refuses unless approved + green + mergeable), then close the Jira ticket |

Try a small bugfix ticket first, or do a no-Jira smoke test with `--local` ([SETUP.md §7](docs/SETUP.md)).

---

## Project layout

```
coding-agent/
├── plugin/                       # the Claude Code plugin
│   ├── .claude-plugin/plugin.json
│   ├── .mcp.json                 # MCP servers: jira-gateway, cks, chainbench
│   ├── commands/                 # /work /review /status /merge /bench
│   ├── agents/                   # orchestrator, planner, implementer, evaluator
│   ├── skills/                   # state-machine, template-parse, pr-sanitize, invariants, …
│   └── hooks/                    # transcript + commit logging
├── contract/                     # agent-mcp.schema.json (tool SSoT) + drift lint
├── tools/jira-gateway-mcp/       # Go MCP: Jira proxy with sensitive-info filter
├── shared/patterns.json          # sensitive-information policy
├── bench/                        # 3-way (cks / code-only / code+skills) comparison harness
└── docs/                         # SETUP.md, system contract, design specs
```

---

## Documentation

- **[docs/SETUP.md](docs/SETUP.md)** — full build, configure, index, and smoke-test guide
- **[HANDOFF.md](HANDOFF.md)** — cross-session context: architecture, design decisions, roadmap
- **[contract/agent-mcp.schema.json](contract/agent-mcp.schema.json)** — the agent-facing tool contract
- **[docs/system-contract.md](docs/system-contract.md)** — the keystone system contract

## License

Apache-2.0 — see [LICENSE](LICENSE).
