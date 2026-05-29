// Package types defines shared data structures for the cks-mcp server.
package types

import "time"

// ------------- Sensitive filter (shared shape with jira-gateway-mcp) -------------

// Severity levels used by filter patterns.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityWarning  Severity = "warning"
)

// FilterAction is what the engine does with a matched range.
type FilterAction string

const (
	ActionBlock  FilterAction = "block"
	ActionRedact FilterAction = "redact"
	ActionWarn   FilterAction = "warn"
)

// ScanResult is the aggregate outcome of a filter scan.
type ScanResult string

const (
	ScanClean    ScanResult = "CLEAN"
	ScanRedacted ScanResult = "REDACTED"
	ScanBlocked  ScanResult = "BLOCKED"
)

// Pattern is the on-disk representation of a single filter rule.
type Pattern struct {
	ID              string       `json:"id"`
	Name            string       `json:"name"`
	Regex           string       `json:"regex,omitempty"`
	Type            string       `json:"type,omitempty"`
	Threshold       float64      `json:"threshold,omitempty"`
	MinLength       int          `json:"min_length,omitempty"`
	MaxLength       int          `json:"max_length,omitempty"`
	Severity        Severity     `json:"severity"`
	Action          FilterAction `json:"action"`
	ExcludePatterns []string     `json:"exclude_patterns,omitempty"`
	Description     string       `json:"description,omitempty"`
}

// IsEntropy reports whether the pattern is an entropy-based detector.
func (p Pattern) IsEntropy() bool { return p.Type == "entropy" }

// PatternsConfig is the top-level config block in patterns.json.
type PatternsConfig struct {
	BlockBehavior     string `json:"block_behavior"`
	RedactReplacement string `json:"redact_replacement"`
	WarnBehavior      string `json:"warn_behavior"`
	MaxScanSizeBytes  int    `json:"max_scan_size_bytes"`
}

// PatternsFile is the root JSON document loaded from shared/patterns.json.
type PatternsFile struct {
	Version  string         `json:"version"`
	Patterns []Pattern      `json:"patterns"`
	Config   PatternsConfig `json:"config"`
}

// FilterMatch describes a matched range within a scanned text.
type FilterMatch struct {
	PatternID string
	Severity  Severity
	Action    FilterAction
	Start     int
	End       int
}

// FilterMetadata is exposed to callers so the agent can branch on scan_result.
type FilterMetadata struct {
	ScanResult       ScanResult `json:"scan_result"`
	RedactedCount    int        `json:"redacted_count"`
	RedactedPatterns []string   `json:"redacted_patterns"`
	BlockedPatterns  []string   `json:"blocked_patterns"`
	Warnings         []string   `json:"warnings"`
	ScannedAt        string     `json:"scanned_at"`
}

// FilterResult is the engine's return value.
type FilterResult struct {
	Text     string
	Metadata FilterMetadata
}

// ------------- CKV chunk + search types -------------

// SymbolType enumerates the recognized Go declaration kinds we chunk on.
type SymbolType string

const (
	SymbolFunction  SymbolType = "function"
	SymbolMethod    SymbolType = "method"
	SymbolStruct    SymbolType = "struct"
	SymbolInterface SymbolType = "interface"
	SymbolConst     SymbolType = "const"
	SymbolVar       SymbolType = "var"
)

// CodeChunk is the unit produced by the AST chunker and consumed by the
// vector store + indexer. Stable across CKV and CKG (Phase 4).
type CodeChunk struct {
	ID           string     `json:"id"`            // sha256(file_path + symbol_name + part_idx)[:16]
	FilePath     string     `json:"file_path"`     // repo-relative
	PackageName  string     `json:"package_name"`  // resolved Go package
	SymbolName   string     `json:"symbol_name"`   // qualified, e.g. "(*Engine).Finalize"
	SymbolType   SymbolType `json:"symbol_type"`
	Code         string     `json:"code"`          // source slice
	Signature    string     `json:"signature"`     // funcDecl signature only
	Godoc        string     `json:"godoc"`         // associated doc comment
	StartLine    int        `json:"start_line"`
	EndLine      int        `json:"end_line"`
	ReceiverType string     `json:"receiver_type,omitempty"` // methods only
	Params       []string   `json:"params,omitempty"`
	Returns      []string   `json:"returns,omitempty"`
	Imports      []string   `json:"imports,omitempty"`
	CodeHash     string     `json:"code_hash"`     // sha256 of Code; used by indexer cache (RI-23)
	IndexedAt    time.Time  `json:"indexed_at"`
	GitModified  string     `json:"git_modified,omitempty"`
	GitAuthor    string     `json:"git_author,omitempty"`
}

// SearchFilters are optional constraints applied during ckv_search.
type SearchFilters struct {
	Package       string `json:"package,omitempty"`
	FilePattern   string `json:"file_pattern,omitempty"`
	SymbolType    string `json:"symbol_type,omitempty"`
	ModifiedSince string `json:"modified_since,omitempty"`
}

// SearchResult is one candidate returned by the search pipeline.
type SearchResult struct {
	FilePath          string   `json:"file"`
	Package           string   `json:"package"`
	Symbol            string   `json:"symbol"`
	SymbolType        string   `json:"symbol_type"`
	Signature         string   `json:"signature"`
	Snippet           string   `json:"snippet"`             // truncated to 500 chars
	Godoc             string   `json:"godoc,omitempty"`
	Score             float64  `json:"score"`
	StartLine         int      `json:"start_line"`
	EndLine           int      `json:"end_line"`
	GitHistorySummary string   `json:"git_history_summary,omitempty"`
	// internal: not serialized to MCP response
	Imports []string `json:"-"`
}

// SearchResponse is the structured payload returned by ckv_search.
type SearchResponse struct {
	Results  []SearchResult `json:"results"`
	Metadata SearchMetadata `json:"metadata"`
}

// SearchMetadata reports pipeline-level stats.
type SearchMetadata struct {
	TotalCandidates int    `json:"total_candidates"`
	Reranked        bool   `json:"reranked"`
	IndexCommit     string `json:"index_commit,omitempty"`
	QueryTimeMs     int64  `json:"query_time_ms"`
	EmbedderMode    string `json:"embedder_mode"` // "vector" or "bm25_fallback" (RI-08)
}

// IndexStats reports the result of an indexing run.
type IndexStats struct {
	FilesProcessed int    `json:"files_processed"`
	ChunksCreated  int    `json:"chunks_created"`
	ChunksUpdated  int    `json:"chunks_updated"`
	ChunksReused   int    `json:"chunks_reused"` // RI-23: from cache
	ChunksDeleted  int    `json:"chunks_deleted"`
	DurationMs     int64  `json:"duration_ms"`
	IndexCommit    string `json:"index_commit,omitempty"`
}

// IndexMeta is persisted alongside the SQLite DB.
type IndexMeta struct {
	Version      string    `json:"version"`
	IndexedAt    time.Time `json:"indexed_at"`
	IndexCommit  string    `json:"index_commit"`
	TotalChunks  int       `json:"total_chunks"`
	Files        int       `json:"files"`
	EmbedderMode string    `json:"embedder_mode"`
	Dimension    int       `json:"dimension"`
}

// ------------- CKG (Code Knowledge Graph) types -------------

// RelationType enumerates the edge kinds we extract from Go AST.
type RelationType string

const (
	RelCalls       RelationType = "calls"
	RelImplements  RelationType = "implements"
	RelUsesType    RelationType = "uses_type"
	RelEmbeds      RelationType = "embeds"
	RelReadsField  RelationType = "reads_field"
	RelWritesField RelationType = "writes_field"
	RelChannels    RelationType = "channels"
)

// ConfidenceLevel reports how reliable a CKG extraction is.
// RI-10/11: when type info is missing or the pattern is interface-dispatch,
// confidence drops so the consumer can adjust its decisions.
type ConfidenceLevel string

const (
	ConfidenceHigh    ConfidenceLevel = "high"
	ConfidenceMedium  ConfidenceLevel = "medium"
	ConfidenceLow     ConfidenceLevel = "low"
	ConfidenceUnknown ConfidenceLevel = "unknown"
)

// GraphNode is a code symbol in the CKG.
type GraphNode struct {
	ID            string     `json:"id"`             // sha256(package + qualified_name)[:16]
	FilePath      string     `json:"file_path"`
	PackageName   string     `json:"package_name"`
	SymbolName    string     `json:"symbol_name"`
	SymbolType    SymbolType `json:"symbol_type"`
	QualifiedName string     `json:"qualified_name"` // "pkg.(*T).Method"
	Signature     string     `json:"signature,omitempty"`
	CodeSnippet   string     `json:"code_snippet,omitempty"` // up to ~20 lines
	StartLine     int        `json:"start_line"`
	EndLine       int        `json:"end_line"`
	IndexedAt     time.Time  `json:"indexed_at"`
}

// GraphEdge connects two graph nodes by a typed relationship.
type GraphEdge struct {
	FromNode     string         `json:"from"`
	ToNode       string         `json:"to"`
	RelationType RelationType   `json:"relation_type"`
	Confidence   ConfidenceLevel `json:"confidence"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// SymbolHistoryEntry is one git commit touching a node.
type SymbolHistoryEntry struct {
	NodeID        string `json:"node_id"`
	CommitHash    string `json:"hash"`
	CommitMessage string `json:"message"`
	CommitDate    string `json:"date"`
	Author        string `json:"author"`
	DiffSummary   string `json:"diff_summary"`
	ChangeType    string `json:"change_type"` // bugfix|feature|refactor|test|change
}

// ChannelOperation describes one send / receive on a channel.
type ChannelOperation struct {
	Channel   string `json:"channel"`
	Direction string `json:"direction"` // "send" | "receive" | "both"
	Type      string `json:"type"`
}

// SyncMechanism reports a synchronization primitive observed in a node.
type SyncMechanism struct {
	Type     string `json:"type"` // mutex | rwmutex | waitgroup | context | atomic | select
	Variable string `json:"variable,omitempty"`
	Scope    string `json:"scope"` // "entire_function" | "partial"
}

// SharedResource describes a struct field touched by multiple goroutines.
type SharedResource struct {
	Resource       string   `json:"resource"`
	AccessType     string   `json:"access_type"` // "read" | "write" | "read_write"
	ProtectedBy    string   `json:"protected_by,omitempty"`
	OtherAccessors []string `json:"other_accessors,omitempty"`
}

// RiskAssessment summarizes race-condition risk for a node.
type RiskAssessment struct {
	RaceConditionRisk        string   `json:"race_condition_risk"` // none|low|medium|high|unknown
	UnprotectedSharedAccess  []string `json:"unprotected_shared_access,omitempty"`
	DeadlockPotential        []string `json:"deadlock_potential,omitempty"`
	Note                     string   `json:"note,omitempty"`
}

// ConcurrencyContext captures everything CKG knows about a node's concurrency
// behaviour. Fields without information are omitted from JSON output.
type ConcurrencyContext struct {
	NodeID             string             `json:"node_id"`
	GoroutineContext   GoroutineContext   `json:"goroutine_context"`
	SharedResources    []SharedResource   `json:"shared_resources,omitempty"`
	ChannelOperations  []ChannelOperation `json:"channel_operations,omitempty"`
	SyncMechanisms     []SyncMechanism    `json:"sync_mechanisms,omitempty"`
	Risk               RiskAssessment     `json:"risk_assessment"`
	Confidence         ConfidenceLevel    `json:"confidence"`
}

// GoroutineContext lists callers/callees that involve goroutine launches.
type GoroutineContext struct {
	LaunchedBy []string `json:"launched_by,omitempty"`
	Launches   []string `json:"launches,omitempty"`
}

// CKGQueryResult is the structured output of ckg_query.
type CKGQueryResult struct {
	Nodes              []GraphNode          `json:"nodes"`
	Edges              []GraphEdge          `json:"edges"`
	History            []SymbolHistoryEntry `json:"history,omitempty"`
	ConcurrencyImpact  []ConcurrencyContext `json:"concurrency_impact,omitempty"`
	Metadata           CKGQueryMetadata     `json:"metadata"`
}

// CKGQueryMetadata reports query-level stats.
type CKGQueryMetadata struct {
	TotalNodes  int   `json:"total_nodes"`
	TotalEdges  int   `json:"total_edges"`
	Truncated   bool  `json:"truncated"`
	QueryTimeMs int64 `json:"query_time_ms"`
}

// CKGImpactResult is the structured output of ckg_impact.
type CKGImpactResult struct {
	Symbol                string           `json:"symbol"`
	DirectCallers         []string         `json:"direct_callers"`
	IndirectCallers       []string         `json:"indirect_callers"`
	InterfaceContracts    []string         `json:"interface_contracts"`
	TestFiles             []string         `json:"test_files"`
	ConcurrencyRisk       ConcurrencyImpact `json:"concurrency_risk"`
	RecommendedTestScope  []string         `json:"recommended_test_scope"`
	RiskLevel             string           `json:"risk_level"` // low|medium|high|critical
	RiskExplanation       string           `json:"risk_explanation"`
}

// ConcurrencyImpact summarizes shared resource impact for ckg_impact.
type ConcurrencyImpact struct {
	AffectedGoroutines       []string `json:"affected_goroutines"`
	SharedResourceConflicts  []string `json:"shared_resource_conflicts"`
}

// CKGIndexStats reports a CKG indexing run.
type CKGIndexStats struct {
	NodesCreated       int   `json:"nodes_created"`
	EdgesCreated       int   `json:"edges_created"`
	HistoryEntries     int   `json:"history_entries"`
	ConcurrencyContexts int  `json:"concurrency_contexts"`
	DurationMs         int64 `json:"duration_ms"`
	Mode               string `json:"mode"` // "typed" or "ast_only" (RI-10)
}
