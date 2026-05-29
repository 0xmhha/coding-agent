# cks-mcp

Code Knowledge Search (CKS) MCP server for the `coding-agent` plugin.

Phase 3 implements **CKV** — vector + lexical code search over the go-stablenet
codebase. Phase 4 will add **CKG** — relation graph, history, concurrency.

## Tools

| Tool | Purpose |
|------|---------|
| `ckv_search` | Semantic search (Ollama vectors) with BM25 fallback when Ollama is unavailable (RI-08). Returns top-k chunks with snippet, signature, godoc, and score. |
| `ckv_index` | Build / refresh the SQLite index. Modes: `full` (project walk) and `incremental` (git diff from last commit). |

## Pipeline overview

```
ckv_index → ParseFile (go/ast) → CodeChunk → Embedder → SQLite store
                                                ↑
                                  code_hash cache (RI-23)

ckv_search → embed query → VectorSearch (cosine, RI-07 brute-force)
                       ↓
                 BM25 fallback (no embedder)
                       ↓
                 Reranker (signature/godoc/recency/package boosts)
                       ↓
                 Sensitive filter (BLOCKED entries dropped)
```

## Design notes

### Brute-force cosine (RI-07)

`modernc.org/sqlite` is the CGo-free driver; `sqlite-vss` requires CGo and is
incompatible. For go-stablenet's ~20k chunks, a single linear scan of float32
BLOBs runs in tens of milliseconds — well within budget.

### Ollama + BM25 fallback (RI-08)

`SelectEmbedder` probes the Ollama server on startup; on failure the server
boots in BM25 mode. Indexing and search both adapt transparently; the
`embedder_mode` field in responses reports which path was used.

### code_hash cache (RI-23)

Each chunk's normalized source is hashed (SHA-256, 16 hex chars). On
incremental runs the indexer skips chunks whose hash matches the stored value
— avoiding unnecessary embedding calls, which are the slow path on CPU.

### Sensitive filter

CKV ships its own copy of the filter logic shared with `jira-gateway-mcp`
(porting via `CKS_PATTERNS_PATH` env var). Search results whose snippets
BLOCK on scan are dropped entirely; REDACTED snippets are returned sanitized.

## Layout

```
tools/cks-mcp/
├── cmd/server/main.go     # stdio MCP server entrypoint
├── internal/
│   ├── ckv/               # chunker, embedder, BM25, store, reranker, search, indexer
│   ├── filter/            # sensitive content filter (shared with jira-gateway-mcp)
│   ├── server/            # MCP tool registration
│   └── types/             # shared types
└── README.md
```

## Build

```bash
go build -o bin/cks-server ./cmd/server
```

## Test

```bash
go test ./...
```

## Run (manual)

```bash
export CKS_INDEX_PATH=.coding-agent/index/ckv.db
./bin/cks-server
```

The server speaks the MCP protocol over stdio; it is intended to be launched
by Claude Code via the plugin's `plugin/.mcp.json` registration.

## Environment variables

| Variable | Description |
|----------|-------------|
| `CKS_INDEX_PATH` | Path to SQLite index file (defaults to `.coding-agent/index/ckv.db`) |
| `CKS_PATTERNS_PATH` | Path to `shared/patterns.json` (auto-detected otherwise) |
| `CUSTOM_PATTERNS_PATH` | Optional override pattern file |
| `OLLAMA_BASE_URL` | Ollama server URL (default `http://localhost:11434`) |
| `OLLAMA_EMBED_MODEL` | Embedding model name (default `nomic-embed-text`) |
| `CKS_DISABLE_OLLAMA` | Set to `1` to force BM25 fallback |
