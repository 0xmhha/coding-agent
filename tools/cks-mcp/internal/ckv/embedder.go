package ckv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

// Embedder produces a fixed-dimensional vector from a piece of text.
// Implementations must be safe for concurrent use.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Dimension() int
	Name() string
}

// FormatChunkForEmbedding builds the canonical text representation a chunk
// is embedded as. Includes package + path + signature + godoc so the embedder
// has structural context, not just raw code.
func FormatChunkForEmbedding(c types.CodeChunk) string {
	var sb bytes.Buffer
	fmt.Fprintf(&sb, "Package: %s\n", c.PackageName)
	fmt.Fprintf(&sb, "File: %s\n", c.FilePath)
	fmt.Fprintf(&sb, "Type: %s\n", c.SymbolType)
	if c.Signature != "" {
		fmt.Fprintf(&sb, "Signature: %s\n", c.Signature)
	}
	if c.Godoc != "" {
		fmt.Fprintf(&sb, "%s\n", c.Godoc)
	}
	sb.WriteString("\n")
	sb.WriteString(c.Code)
	return sb.String()
}

// --- Ollama embedder (Tier 1: local) ---

// OllamaEmbedder calls a local Ollama server's /api/embeddings endpoint.
// Default model is nomic-embed-text (768 dimensions).
type OllamaEmbedder struct {
	baseURL string
	model   string
	dim     int
	http    *http.Client
}

// OllamaOptions configures the embedder.
type OllamaOptions struct {
	BaseURL string // defaults to OLLAMA_BASE_URL or http://localhost:11434
	Model   string // defaults to OLLAMA_EMBED_MODEL or nomic-embed-text
	Timeout time.Duration
}

// NewOllamaEmbedder constructs an Ollama embedder. The embedder is created
// in a deferred way: a single probe call is issued to verify the server is
// reachable and to discover the embedding dimension. If probing fails, this
// returns a sentinel error so callers can fall back to BM25 (RI-08).
func NewOllamaEmbedder(ctx context.Context, opts OllamaOptions) (*OllamaEmbedder, error) {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = os.Getenv("OLLAMA_BASE_URL")
	}
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	model := opts.Model
	if model == "" {
		model = os.Getenv("OLLAMA_EMBED_MODEL")
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	e := &OllamaEmbedder{
		baseURL: baseURL,
		model:   model,
		http:    &http.Client{Timeout: timeout},
	}

	// Probe with a tiny payload to verify the server + model + measure dim.
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	vec, err := e.embedOnce(probeCtx, "ping")
	if err != nil {
		return nil, fmt.Errorf("ollama probe failed: %w", err)
	}
	if len(vec) == 0 {
		return nil, errors.New("ollama probe returned empty embedding")
	}
	e.dim = len(vec)
	return e, nil
}

// Name returns the embedder identifier.
func (e *OllamaEmbedder) Name() string { return "ollama:" + e.model }

// Dimension returns the embedding dimension discovered during the probe.
func (e *OllamaEmbedder) Dimension() int { return e.dim }

// Embed returns the embedding vector for text.
func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return e.embedOnce(ctx, text)
}

func (e *OllamaEmbedder) embedOnce(ctx context.Context, prompt string) ([]float32, error) {
	body, err := json.Marshal(map[string]any{
		"model":  e.model,
		"prompt": prompt,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.baseURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("ollama %d: %s", resp.StatusCode, string(snippet))
	}
	var payload struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}
	return payload.Embedding, nil
}

// --- Fallback selection helper ---

// SelectEmbedder tries to construct the local Ollama embedder and returns
// it on success. If Ollama is unreachable, it returns (nil, err) so the
// caller can switch to BM25 mode (RI-08).
func SelectEmbedder(ctx context.Context) (Embedder, error) {
	e, err := NewOllamaEmbedder(ctx, OllamaOptions{})
	if err != nil {
		return nil, err
	}
	return e, nil
}
