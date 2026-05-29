package ckv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

func writeTempGo(t *testing.T, name, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestParseFile_FunctionWithDoc(t *testing.T) {
	src := `package wbft

// Finalize seals the block.
func Finalize(x int) error { return nil }
`
	path := writeTempGo(t, "engine.go", src)
	chunks, err := ParseFile(filepath.Dir(path), path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks; want 1", len(chunks))
	}
	c := chunks[0]
	if c.SymbolType != types.SymbolFunction {
		t.Fatalf("SymbolType = %s; want function", c.SymbolType)
	}
	if c.PackageName != "wbft" {
		t.Fatalf("PackageName = %q; want wbft", c.PackageName)
	}
	if c.SymbolName != "Finalize" {
		t.Fatalf("SymbolName = %q; want Finalize", c.SymbolName)
	}
	if !strings.Contains(c.Signature, "Finalize") {
		t.Fatalf("Signature = %q; want to contain Finalize", c.Signature)
	}
	if !strings.Contains(c.Godoc, "Finalize seals the block") {
		t.Fatalf("Godoc = %q; want to contain doc text", c.Godoc)
	}
	if c.CodeHash == "" {
		t.Fatalf("CodeHash empty")
	}
	if len(c.Params) != 1 || c.Params[0] != "int" {
		t.Fatalf("Params = %v; want [int]", c.Params)
	}
	if len(c.Returns) != 1 || c.Returns[0] != "error" {
		t.Fatalf("Returns = %v; want [error]", c.Returns)
	}
}

func TestParseFile_Method(t *testing.T) {
	src := `package wbft

type Engine struct{}

func (e *Engine) Finalize() error { return nil }
`
	path := writeTempGo(t, "engine.go", src)
	chunks, err := ParseFile(filepath.Dir(path), path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	var method *types.CodeChunk
	for i := range chunks {
		if chunks[i].SymbolType == types.SymbolMethod {
			method = &chunks[i]
			break
		}
	}
	if method == nil {
		t.Fatalf("no method chunk found; chunks = %+v", chunks)
	}
	if method.ReceiverType != "Engine" {
		t.Fatalf("ReceiverType = %q; want Engine", method.ReceiverType)
	}
	if method.SymbolName != "(Engine).Finalize" {
		t.Fatalf("SymbolName = %q; want (Engine).Finalize", method.SymbolName)
	}
}

func TestParseFile_StructAndInterface(t *testing.T) {
	src := `package wbft

// Config holds engine config.
type Config struct {
	X int
	Y string
}

// Sealer is something that seals.
type Sealer interface {
	Seal() error
}
`
	path := writeTempGo(t, "types.go", src)
	chunks, err := ParseFile(filepath.Dir(path), path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	gotStruct := false
	gotIface := false
	for _, c := range chunks {
		if c.SymbolType == types.SymbolStruct && c.SymbolName == "Config" {
			gotStruct = true
		}
		if c.SymbolType == types.SymbolInterface && c.SymbolName == "Sealer" {
			gotIface = true
		}
	}
	if !gotStruct {
		t.Fatalf("expected Config struct chunk")
	}
	if !gotIface {
		t.Fatalf("expected Sealer interface chunk")
	}
}

func TestParseFile_LargeFunctionSplits(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("package big\n\nfunc Big() {\n")
	for i := 0; i < 220; i++ {
		sb.WriteString("\t_ = " + "1\n")
	}
	sb.WriteString("}\n")

	path := writeTempGo(t, "big.go", sb.String())
	chunks, err := ParseFile(filepath.Dir(path), path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(chunks) <= 1 {
		t.Fatalf("expected >1 sub-chunks for large function; got %d", len(chunks))
	}
	for _, c := range chunks {
		if c.Signature == "" {
			t.Fatalf("sub-chunk missing parent signature")
		}
	}
}

func TestParseProject_ExcludesAndIncludes(t *testing.T) {
	dir := t.TempDir()
	must := func(p, src string) {
		t.Helper()
		full := filepath.Join(dir, p)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(src), 0o600); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	must("a.go", "package a\nfunc A(){}\n")
	must("a_test.go", "package a\nfunc TestA(){}\n")
	must("a_gen.go", "package a\nfunc Gen(){}\n")
	must("vendor/dep/x.go", "package dep\nfunc Vendored(){}\n")
	must(".hidden/x.go", "package x\nfunc Hidden(){}\n")

	opts := DefaultOptions()
	chunks, err := ParseProject(dir, opts)
	if err != nil {
		t.Fatalf("ParseProject: %v", err)
	}
	got := map[string]bool{}
	for _, c := range chunks {
		got[c.SymbolName] = true
	}
	if !got["A"] {
		t.Fatalf("expected A in chunks")
	}
	if !got["TestA"] {
		t.Fatalf("expected TestA in chunks (IncludeTests=true)")
	}
	if got["Gen"] {
		t.Fatalf("did not expect Gen (excluded by _gen.go pattern)")
	}
	if got["Vendored"] {
		t.Fatalf("did not expect Vendored (excluded by vendor/)")
	}
	if got["Hidden"] {
		t.Fatalf("did not expect Hidden (hidden directory)")
	}
}
