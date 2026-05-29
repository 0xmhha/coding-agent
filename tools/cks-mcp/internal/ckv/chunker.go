// Package ckv implements the Code Knowledge Vector subsystem of cks-mcp:
// AST-based chunking, embedding, storage, search, and indexing.
package ckv

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

const (
	// largeFuncThreshold is the source-line count above which a function is
	// split into sub-chunks.
	largeFuncThreshold = 200
)

// ChunkOptions controls inclusion / exclusion of files during parsing.
type ChunkOptions struct {
	IncludeTests bool     // include *_test.go
	Excludes     []string // glob-ish substring exclusions; e.g. "vendor/", "_gen.go"
}

// DefaultOptions returns options matching the Phase 3 spec.
func DefaultOptions() ChunkOptions {
	return ChunkOptions{
		IncludeTests: true,
		Excludes:     []string{"vendor/", "_gen.go", "_mock.go"},
	}
}

// ParseFile parses a single Go source file and returns its chunks.
// rootDir is used to compute repo-relative file paths.
func ParseFile(rootDir, absPath string) ([]types.CodeChunk, error) {
	fset := token.NewFileSet()
	src, err := os.ReadFile(absPath) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", absPath, err)
	}
	f, err := parser.ParseFile(fset, absPath, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", absPath, err)
	}
	rel, err := filepath.Rel(rootDir, absPath)
	if err != nil {
		rel = absPath
	}
	rel = filepath.ToSlash(rel)

	imports := collectImports(f)
	pkgName := ""
	if f.Name != nil {
		pkgName = f.Name.Name
	}

	now := time.Now().UTC()
	var chunks []types.CodeChunk
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			cs := chunkFuncDecl(fset, src, d, rel, pkgName, imports, now)
			chunks = append(chunks, cs...)
		case *ast.GenDecl:
			cs := chunkGenDecl(fset, src, d, rel, pkgName, imports, now)
			chunks = append(chunks, cs...)
		}
	}
	return chunks, nil
}

// ParseProject walks the project tree and returns chunks for all matching
// Go files. Files are filtered per opts and the default Go conventions
// (vendor/, hidden directories).
func ParseProject(rootDir string, opts ChunkOptions) ([]types.CodeChunk, error) {
	var all []types.CodeChunk

	walkErr := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip hidden dirs (.git, .coding-agent, etc) but keep root.
		if d.IsDir() {
			name := d.Name()
			if path != rootDir && strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if !opts.IncludeTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for _, ex := range opts.Excludes {
			if strings.Contains(path, ex) {
				return nil
			}
		}
		chunks, perr := ParseFile(rootDir, path)
		if perr != nil {
			// One bad file shouldn't kill the entire scan.
			fmt.Fprintf(os.Stderr, "[cks-mcp] warning: skip %s: %v\n", path, perr)
			return nil
		}
		all = append(all, chunks...)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return all, nil
}

// --- internal helpers ---

func collectImports(f *ast.File) []string {
	out := make([]string, 0, len(f.Imports))
	for _, imp := range f.Imports {
		if imp.Path == nil {
			continue
		}
		out = append(out, strings.Trim(imp.Path.Value, `"`))
	}
	return out
}

func chunkFuncDecl(
	fset *token.FileSet,
	src []byte,
	fn *ast.FuncDecl,
	relPath, pkgName string,
	fileImports []string,
	now time.Time,
) []types.CodeChunk {
	startLine, endLine := position(fset, fn.Pos(), fn.End())
	code := sliceSource(src, fset, fn.Pos(), fn.End())
	signature := buildFuncSignature(fset, fn)
	godoc := docCommentText(fn.Doc)
	recvType, isMethod := receiverTypeName(fn)
	params := buildParamTypes(fset, fn.Type)
	returns := buildReturnTypes(fset, fn.Type)

	symbol := fn.Name.Name
	if isMethod {
		symbol = fmt.Sprintf("(%s).%s", recvType, fn.Name.Name)
	}

	symbolType := types.SymbolFunction
	if isMethod {
		symbolType = types.SymbolMethod
	}

	// Large function: split into sub-chunks by top-level block.
	if endLine-startLine+1 > largeFuncThreshold && fn.Body != nil {
		return splitLargeFunc(fset, src, fn, relPath, pkgName, fileImports,
			symbol, signature, godoc, recvType, params, returns, symbolType, now)
	}

	chunk := types.CodeChunk{
		ID:           chunkID(relPath, symbol, 0),
		FilePath:     relPath,
		PackageName:  pkgName,
		SymbolName:   symbol,
		SymbolType:   symbolType,
		Code:         code,
		Signature:    signature,
		Godoc:        godoc,
		StartLine:    startLine,
		EndLine:      endLine,
		ReceiverType: recvType,
		Params:       params,
		Returns:      returns,
		Imports:      fileImports,
		CodeHash:     hashCode(code),
		IndexedAt:    now,
	}
	return []types.CodeChunk{chunk}
}

func splitLargeFunc(
	fset *token.FileSet,
	src []byte,
	fn *ast.FuncDecl,
	relPath, pkgName string,
	fileImports []string,
	symbol, signature, godoc, recvType string,
	params, returns []string,
	symbolType types.SymbolType,
	now time.Time,
) []types.CodeChunk {
	stmts := fn.Body.List
	if len(stmts) == 0 {
		// Pathological: declared as large but empty. Fall through to single chunk.
		startLine, endLine := position(fset, fn.Pos(), fn.End())
		code := sliceSource(src, fset, fn.Pos(), fn.End())
		return []types.CodeChunk{{
			ID:        chunkID(relPath, symbol, 0),
			FilePath:  relPath, PackageName: pkgName,
			SymbolName: symbol, SymbolType: symbolType,
			Code: code, Signature: signature, Godoc: godoc,
			StartLine: startLine, EndLine: endLine,
			ReceiverType: recvType, Params: params, Returns: returns,
			Imports: fileImports, CodeHash: hashCode(code), IndexedAt: now,
		}}
	}

	// One sub-chunk per top-level stmt. The full body is reconstructed for each.
	out := make([]types.CodeChunk, 0, len(stmts))
	parentNote := fmt.Sprintf("// Part of: %s\n", signature)

	for i, stmt := range stmts {
		startLine, endLine := position(fset, stmt.Pos(), stmt.End())
		code := sliceSource(src, fset, stmt.Pos(), stmt.End())
		if godoc != "" && i == 0 {
			code = parentNote + godoc + "\n" + code
		} else {
			code = parentNote + code
		}
		subSymbol := fmt.Sprintf("%s#part%d", symbol, i+1)
		out = append(out, types.CodeChunk{
			ID:           chunkID(relPath, symbol, i+1),
			FilePath:     relPath,
			PackageName:  pkgName,
			SymbolName:   subSymbol,
			SymbolType:   symbolType,
			Code:         code,
			Signature:    signature,
			Godoc:        godoc,
			StartLine:    startLine,
			EndLine:      endLine,
			ReceiverType: recvType,
			Params:       params,
			Returns:      returns,
			Imports:      fileImports,
			CodeHash:     hashCode(code),
			IndexedAt:    now,
		})
	}
	return out
}

func chunkGenDecl(
	fset *token.FileSet,
	src []byte,
	d *ast.GenDecl,
	relPath, pkgName string,
	fileImports []string,
	now time.Time,
) []types.CodeChunk {
	switch d.Tok {
	case token.TYPE:
		return chunkTypeDecl(fset, src, d, relPath, pkgName, fileImports, now)
	case token.CONST, token.VAR:
		return chunkValueDecl(fset, src, d, relPath, pkgName, fileImports, now)
	default:
		return nil
	}
}

func chunkTypeDecl(
	fset *token.FileSet,
	src []byte,
	d *ast.GenDecl,
	relPath, pkgName string,
	fileImports []string,
	now time.Time,
) []types.CodeChunk {
	godoc := docCommentText(d.Doc)
	var chunks []types.CodeChunk
	for _, spec := range d.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok || ts.Name == nil {
			continue
		}
		startLine, endLine := position(fset, ts.Pos(), ts.End())
		code := sliceSource(src, fset, ts.Pos(), ts.End())
		st := types.SymbolStruct
		switch ts.Type.(type) {
		case *ast.InterfaceType:
			st = types.SymbolInterface
		case *ast.StructType:
			st = types.SymbolStruct
		}
		chunks = append(chunks, types.CodeChunk{
			ID:          chunkID(relPath, ts.Name.Name, 0),
			FilePath:    relPath,
			PackageName: pkgName,
			SymbolName:  ts.Name.Name,
			SymbolType:  st,
			Code:        code,
			Signature:   "type " + ts.Name.Name,
			Godoc:       godoc,
			StartLine:   startLine,
			EndLine:     endLine,
			Imports:     fileImports,
			CodeHash:    hashCode(code),
			IndexedAt:   now,
		})
	}
	return chunks
}

func chunkValueDecl(
	fset *token.FileSet,
	src []byte,
	d *ast.GenDecl,
	relPath, pkgName string,
	fileImports []string,
	now time.Time,
) []types.CodeChunk {
	if len(d.Specs) == 0 {
		return nil
	}
	startLine, endLine := position(fset, d.Pos(), d.End())
	code := sliceSource(src, fset, d.Pos(), d.End())
	godoc := docCommentText(d.Doc)

	// Use the first declared name as the symbol; group declarations stay together.
	first := firstValueName(d)
	if first == "" {
		first = fmt.Sprintf("anon_%d", startLine)
	}
	st := types.SymbolConst
	if d.Tok == token.VAR {
		st = types.SymbolVar
	}

	return []types.CodeChunk{{
		ID:          chunkID(relPath, first, 0),
		FilePath:    relPath,
		PackageName: pkgName,
		SymbolName:  first,
		SymbolType:  st,
		Code:        code,
		Signature:   d.Tok.String() + " " + first,
		Godoc:       godoc,
		StartLine:   startLine,
		EndLine:     endLine,
		Imports:     fileImports,
		CodeHash:    hashCode(code),
		IndexedAt:   now,
	}}
}

func firstValueName(d *ast.GenDecl) string {
	for _, s := range d.Specs {
		vs, ok := s.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, n := range vs.Names {
			if n.Name != "" {
				return n.Name
			}
		}
	}
	return ""
}

func position(fset *token.FileSet, start, end token.Pos) (int, int) {
	s := fset.Position(start).Line
	e := fset.Position(end).Line
	if e < s {
		e = s
	}
	return s, e
}

func sliceSource(src []byte, fset *token.FileSet, start, end token.Pos) string {
	startOff := fset.Position(start).Offset
	endOff := fset.Position(end).Offset
	if startOff < 0 {
		startOff = 0
	}
	if endOff > len(src) {
		endOff = len(src)
	}
	if endOff < startOff {
		return ""
	}
	return string(src[startOff:endOff])
}

func buildFuncSignature(fset *token.FileSet, fn *ast.FuncDecl) string {
	var buf bytes.Buffer
	buf.WriteString("func ")
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		buf.WriteString("(")
		_ = printer.Fprint(&buf, fset, fn.Recv.List[0].Type)
		buf.WriteString(") ")
	}
	buf.WriteString(fn.Name.Name)
	if fn.Type != nil {
		var typeBuf bytes.Buffer
		_ = printer.Fprint(&typeBuf, fset, fn.Type)
		// printer prints "func(...)"; strip the leading "func".
		sig := strings.TrimPrefix(typeBuf.String(), "func")
		buf.WriteString(sig)
	}
	return collapseWhitespace(buf.String())
}

func buildParamTypes(fset *token.FileSet, ft *ast.FuncType) []string {
	if ft == nil || ft.Params == nil {
		return nil
	}
	return fieldListTypes(fset, ft.Params)
}

func buildReturnTypes(fset *token.FileSet, ft *ast.FuncType) []string {
	if ft == nil || ft.Results == nil {
		return nil
	}
	return fieldListTypes(fset, ft.Results)
}

func fieldListTypes(fset *token.FileSet, fl *ast.FieldList) []string {
	if fl == nil {
		return nil
	}
	out := make([]string, 0, len(fl.List))
	for _, f := range fl.List {
		var buf bytes.Buffer
		_ = printer.Fprint(&buf, fset, f.Type)
		t := buf.String()
		if len(f.Names) == 0 {
			out = append(out, t)
			continue
		}
		// Same type repeated for each name (Go grouping rule).
		for range f.Names {
			out = append(out, t)
		}
	}
	return out
}

func receiverTypeName(fn *ast.FuncDecl) (string, bool) {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return "", false
	}
	t := fn.Recv.List[0].Type
	for {
		star, ok := t.(*ast.StarExpr)
		if !ok {
			break
		}
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name, true
	}
	return "", true
}

func docCommentText(g *ast.CommentGroup) string {
	if g == nil {
		return ""
	}
	var sb strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(g.Text()))
	for scanner.Scan() {
		sb.WriteString(strings.TrimRight(scanner.Text(), " \t"))
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

func collapseWhitespace(s string) string {
	var sb strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if prevSpace {
				continue
			}
			sb.WriteRune(' ')
			prevSpace = true
			continue
		}
		sb.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(sb.String())
}

func chunkID(filePath, symbol string, partIdx int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d", filePath, symbol, partIdx)))
	return hex.EncodeToString(h[:8])
}

func hashCode(code string) string {
	h := sha256.Sum256([]byte(code))
	return hex.EncodeToString(h[:16])
}
