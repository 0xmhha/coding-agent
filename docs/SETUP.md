# Setup Guide

This document gets you from "I just cloned the repo" to "I can run
`/coding-agent:work STABLE-1234` on go-stablenet". Follow the sections in
order; each one ends with a quick verification command so you know it worked.

If something fails, skip to [§9 Troubleshooting](#9-troubleshooting).

---

## 1. Prerequisites

| Tool | Why | Check |
|------|-----|-------|
| Go ≥ 1.25 | Build both MCP servers + your stablenet test runs | `go version` |
| git ≥ 2.40 | Branch/commit/log throughout the pipeline | `git --version` |
| GitHub CLI (`gh`) ≥ 2.50 | PR creation, comments, status checks, merge | `gh auth status` |
| Claude Code | Hosts the plugin | (CLI/IDE) |
| Atlassian (Jira) Cloud account | Source of tickets | (web) |
| ChainBench MCP server | Stage 4 of the Evaluator | (separate install) |
| Ollama + `nomic-embed-text` | Optional: Tier-1 CKV embeddings (RI-08) | `ollama list` |
| Python 3 | Optional: ad-hoc JSON inspection | `python3 --version` |

A note on optionality:

- **Ollama** is optional. Without it, CKV falls back to BM25 (lexical) search.
  The pipeline still works; result quality is just lower.
- **ChainBench** is required for Stage 4 of the Evaluator. If you skip its
  installation, the Evaluator will fail Stage 4 with a clear message
  identifying the missing MCP tools (RI-20).

---

## 2. Clone the repository

```bash
git clone <repo-url> coding-agent
cd coding-agent
```

The directory layout you should see:

```
coding-agent/
├── plugin/                  # Claude Code plugin (commands, agents, skills, hooks)
├── tools/                   # MCP servers (Go projects)
│   ├── cks-mcp/             # Code Knowledge Search (CKV + CKG)
│   └── jira-gateway-mcp/    # Sensitive-filter proxy in front of Jira REST API
├── shared/
│   └── patterns.json        # Sensitive-information policy, shared by both servers
└── docs/                    # Specs and plans
```

---

## 3. Build both MCP servers

```bash
cd tools/jira-gateway-mcp
go build -o bin/jira-gateway-mcp ./cmd/server

cd ../cks-mcp
go build -o bin/cks-server ./cmd/server
```

Verification:

```bash
ls -l tools/jira-gateway-mcp/bin/jira-gateway-mcp
ls -l tools/cks-mcp/bin/cks-server
```

Both files should be ~10–20 MiB executables.

Run the test suites to confirm nothing was broken by your environment:

```bash
cd tools/jira-gateway-mcp && go test ./...
cd ../cks-mcp && go test ./...
```

All packages should print `ok`.

---

## 4. Configure environment variables

The plugin reads its secrets from environment variables that get forwarded
into the MCP servers via `plugin/.mcp.json`. Set them once in your shell
profile (or any process manager you use) so Claude Code's child processes
inherit them.

### 4.1 Jira (required)

Create an API token at
<https://id.atlassian.com/manage-profile/security/api-tokens>.

```bash
export JIRA_BASE_URL="https://your-domain.atlassian.net"
export JIRA_USER_EMAIL="you@example.com"
export JIRA_API_TOKEN="atlassian_api_token_here"
```

Verify by reading a ticket the plugin will care about:

```bash
JIRA_BASE_URL=$JIRA_BASE_URL \
JIRA_API_TOKEN=$JIRA_API_TOKEN \
JIRA_USER_EMAIL=$JIRA_USER_EMAIL \
curl -s -u "$JIRA_USER_EMAIL:$JIRA_API_TOKEN" \
  "$JIRA_BASE_URL/rest/api/3/myself" | python3 -m json.tool | head -5
```

A successful call returns your account info. A 401 means the token or email
is wrong.

### 4.2 CKS index location

The CKS server stores its SQLite index at `CKS_INDEX_PATH`. If left unset
it defaults to `.coding-agent/index/ckv.db` relative to your current working
directory.

```bash
# Recommended: anchor to the project under analysis so the index follows the codebase.
export CKS_INDEX_PATH="$HOME/Work/go-stablenet/.coding-agent/index/ckv.db"
```

### 4.3 Ollama (optional)

If you want Tier-1 embeddings, install Ollama and pull the model:

```bash
brew install ollama        # or per https://ollama.com/download
ollama serve &              # background daemon
ollama pull nomic-embed-text
```

Then point the CKS server at it:

```bash
export OLLAMA_BASE_URL="http://localhost:11434"
export OLLAMA_EMBED_MODEL="nomic-embed-text"
# Leave CKS_DISABLE_OLLAMA unset to enable vector search.
```

To force the BM25 fallback (e.g., for reproducibility in CI), set:

```bash
export CKS_DISABLE_OLLAMA=1
```

Verify Ollama:

```bash
curl -s http://localhost:11434/api/embeddings \
  -d '{"model":"nomic-embed-text","prompt":"hello"}' | head -c 80
```

A JSON array starting with `{"embedding":[…]}` confirms it's working.

### 4.4 ChainBench

Install the ChainBench MCP server per its own instructions. Note the binary
path (or `npx` invocation) so you can register it in your Claude Code's
user-level MCP config. The coding-agent plugin doesn't ship ChainBench
because it is a separate project. Phase 6 Stage 4 will detect missing tools
and fail gracefully (RI-20) so the rest of the pipeline still runs to
completion if you delay the ChainBench setup.

---

## 5. Install the plugin in Claude Code

The plugin lives at `coding-agent/plugin/`. Point Claude Code at it via your
user-level config (or use the marketplace mechanism your installation
supports).

### 5.1 Direct path install (recommended for local development)

In your Claude Code config (typically `~/.claude/config.json`, depending on
client), add:

```jsonc
{
  "plugins": {
    "coding-agent": {
      "path": "/absolute/path/to/coding-agent/plugin"
    }
  }
}
```

Claude Code's plugin loader will discover:

- `plugin/.claude-plugin/plugin.json` (manifest)
- `plugin/commands/*.md` (slash commands)
- `plugin/agents/*.md` (sub-agents)
- `plugin/skills/{name}/SKILL.md` (skills)
- `plugin/hooks/hooks.json` (PostToolUse hooks)
- `plugin/.mcp.json` (MCP server registration)

### 5.2 Verify Claude Code picks it up

Restart Claude Code, open a project, and check:

```
/help
```

You should see `/coding-agent:work`, `/coding-agent:review`,
`/coding-agent:status`, `/coding-agent:merge` in the slash command list.

Open the MCP status panel (or use whatever your client exposes); both
`jira-gateway` and `cks` should show as **connected**.

If either MCP fails to start, check the launching process's env: the
inherited environment must include the variables from §4 because
`.mcp.json` substitutes `${...}` from the parent shell.

---

## 6. First-time CKS indexing

Before the Planner can do anything useful, CKS needs to ingest the
go-stablenet codebase.

### 6.1 Index the project (full mode)

Inside Claude Code, call the `ckv_index` and `ckg_index` MCP tools directly
(via your client's MCP UI, or by asking the LLM to call them):

```jsonc
// ckv_index input
{
  "mode": "full",
  "project_dir": "/absolute/path/to/go-stablenet"
}

// ckg_index input
{
  "project_dir": "/absolute/path/to/go-stablenet"
}
```

Expected wall time on a typical go-stablenet checkout:

| Stage | Ollama installed | BM25 fallback |
|-------|------------------|---------------|
| ckv_index (full) | ~30–60 min (RI-09) | ~5–10 min |
| ckg_index | ~5–10 min | ~5–10 min |

Subsequent runs hit the `code_hash` cache (RI-23) and complete in seconds
for unchanged files. The Planner triggers incremental updates
automatically through `/coding-agent:work`.

### 6.2 Verify the index

Ask CKV to find something you expect to exist:

```jsonc
// ckv_search input
{
  "query": "consensus finalize block",
  "top_k": 5
}
```

You should get back results that mention `consensus/wbft/...` symbols. If
the results are empty or unrelated, re-check the index path and project
directory.

For CKG:

```jsonc
// ckg_query input
{
  "symbols": ["wbft.(*WBFTEngine).Finalize"],
  "depth": 1,
  "include_concurrency": true
}
```

A non-empty `nodes` array indicates the graph was built.

---

## 7. Smoke test the pipeline

### 7.1 Local-mode `/work` (no Jira)

Create a fake ticket file so you can validate Phase 1 without hitting Jira:

```bash
cat > /tmp/test-ticket.json <<'EOF'
{
  "ticket_id": "TEST-1",
  "type": "Bug Fix",
  "summary": "Sample sanity ticket",
  "description": "## 작업 유형: Bug Fix\n## 요약\nNothing to do.\n## 재현 방법\n1. nothing\n## 기대 동작\nworks\n## 실제 동작\nworks\n## 영향 범위\n- 모듈: consensus\n- 심각도: low\n## 수용 기준\n- [ ] sample\n",
  "assignee": null,
  "status": "To Do",
  "status_category": "todo",
  "labels": [],
  "created": "2026-05-29T00:00:00Z",
  "updated": "2026-05-29T00:00:00Z",
  "_filter_metadata": { "scan_result": "CLEAN", "redacted_count": 0,
                        "redacted_patterns": [], "blocked_patterns": [],
                        "warnings": [], "scanned_at": "2026-05-29T00:00:00Z" }
}
EOF
```

Then in Claude Code:

```
/coding-agent:work TEST-1 --local /tmp/test-ticket.json
```

You should see the Orchestrator pick up `TEST-1`, the Planner produce an
`analysis.md`, and the pipeline halt politely when it can't find real code
to modify (or when it asks you to confirm).

### 7.2 Status check

```
/coding-agent:status
```

You should see one active workspace for `TEST-1`. Use the long form to
inspect it:

```
/coding-agent:status TEST-1
```

### 7.3 Cleanup

Active workspaces live under `.coding-agent/tickets/` in the project under
analysis. Delete TEST-1 when you're done:

```bash
rm -rf .coding-agent/tickets/TEST-1_*
```

---

## 8. Wire in your real workflow

Once the smoke test passes:

1. Pick an actual Jira ticket. Try a small bugfix first.
2. Run `/coding-agent:work STABLE-XXXX` without `--local`.
3. Watch the Orchestrator advance through ANALYSIS → PLANNING → DESIGN →
   IMPLEMENTATION → EVALUATION.
4. When the Evaluator reaches Stage 4 (ChainBench), it will fail loudly
   if your ChainBench MCP isn't wired up — that's a configuration
   problem, not a pipeline bug (RI-20).
5. After EVALUATION_PASS, the Orchestrator creates a PR. Review it
   yourself or have a teammate review it.
6. If reviewers leave comments, run `/coding-agent:review <PR-URL>` to
   trigger the review cycle.
7. When ready to merge, run `/coding-agent:merge STABLE-XXXX`. This is
   the only command that touches `main`; it refuses to proceed unless the
   PR is approved, all status checks are green, and it's mergeable.

---

## 9. Troubleshooting

### 9.1 `MCP server 'cks' is not connected`

- Check that the binary exists at the path `.mcp.json` points to.
- Check that the parent shell exported `CKS_INDEX_PATH` and (if you want
  vectors) the Ollama variables.
- Read the server's stderr — Claude Code usually surfaces it in a panel.
  The most common error is `Ollama unavailable`, which is a warning, not
  an error: the server still boots in BM25 mode.

### 9.2 `Jira: authentication failed`

- Re-issue the token: <https://id.atlassian.com/manage-profile/security/api-tokens>.
- Confirm `JIRA_USER_EMAIL` matches the account that owns the token. Some
  organizations also require the token's "site access" to include your
  Atlassian instance.
- Try `curl` from §4.1 to isolate whether the problem is the token or the
  plugin.

### 9.3 `state.json transition blocked`

The pipeline refuses to advance when an artifact is missing or incomplete
(by design, RI-13). The error message lists the missing files. Open the
workspace, fix the artifact (or delete the workspace if it's stale), and
re-run `/coding-agent:work`.

### 9.4 `Ollama probe failed`

Either start Ollama (`ollama serve &`) or accept the BM25 fallback. The
pipeline doesn't care which path you choose, but search relevance is better
with Ollama. Set `CKS_DISABLE_OLLAMA=1` to silence the probe and force
BM25.

### 9.5 `gh pr merge: PR is not mergeable`

The merge command checks three things:

1. PR is approved.
2. All status checks succeeded.
3. GitHub reports `mergeable: MERGEABLE`.

If you're in CHANGES_REQUESTED, run `/coding-agent:review <PR-URL>`. If a
check is failing, look at the gh output for the failing check name. If the
state is CONFLICTING, resolve conflicts on the branch and push the resolution.

### 9.6 `Evaluator Stage 4: ChainBench MCP interface mismatch`

The Phase 6 spec listed expected tool names (`chainbench_setup`,
`chainbench_start`, …). If your ChainBench server uses different names,
update `plugin/agents/evaluator.md §7` to match (RI-20). The Evaluator
detects the mismatch before running so it doesn't leave processes lying
around.

### 9.7 `Filter engine error: patterns.json not found`

The filter engines look for `shared/patterns.json` via:

1. `PATTERNS_PATH` env var (jira-gateway) / `CKS_PATTERNS_PATH` (cks),
2. relative paths derived from the binary location,
3. `./shared/patterns.json` from cwd.

If all three fail the engine fails closed — it returns `BLOCKED` rather
than letting data through unscanned. Set the env var explicitly:

```bash
export PATTERNS_PATH="/absolute/path/to/coding-agent/shared/patterns.json"
export CKS_PATTERNS_PATH="$PATTERNS_PATH"
```

### 9.8 Hooks not firing

The hooks are best-effort logging. They never block the pipeline, and they
fail open. If you don't see entries in `{workspace}/logs/impl.log`, check:

- The hook scripts have the executable bit (`ls -l plugin/hooks/*.sh`).
- The `${CLAUDE_PLUGIN_ROOT}` variable resolves to the plugin install
  path in your Claude Code build.

Hooks are documented as advisory — the pipeline works without them.

---

## 10. What to look at next

- `docs/superpowers/specs/` — the canonical design documents
- `docs/plan/REVIEW_ISSUES.md` — known limitations and how the pipeline
  handles each one (RI-01..23)
- `docs/plan/phase{1..7}-tasks.md` — granular task lists with current
  status
- `tools/{jira-gateway-mcp,cks-mcp}/README.md` — server-level documentation

When you're comfortable with the pipeline on a small ticket, you can scale
up to larger features. The Orchestrator caps automatic retries at
`max_eval_cycles` (default 3) so the pipeline never spins forever — see
the BLOCKED state report and intervene manually when needed.
