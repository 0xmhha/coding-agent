package ckg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	internaltypes "github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"

	"golang.org/x/tools/go/packages"
)

// ExtractMode reports which extraction tier produced the data.
//
// RI-10: when packages.Load succeeds, we use Tier 1 (typed) with high
// confidence; otherwise we fall back to Tier 2 (AST-only) with low confidence.
type ExtractMode string

const (
	ModeTyped   ExtractMode = "typed"
	ModeASTOnly ExtractMode = "ast_only"
)

// ExtractResult is what one analyzer run produces. The store consumes nodes
// and edges directly; mode is reported back so ckg_index can surface it.
type ExtractResult struct {
	Mode  ExtractMode
	Nodes []internaltypes.GraphNode
	Edges []internaltypes.GraphEdge
}

// Extract runs the relation extractor on the project rooted at projectDir.
// It first attempts the typed path; on failure it falls back to AST-only.
func Extract(ctx context.Context, projectDir string) (*ExtractResult, error) {
	if res, err := extractTyped(ctx, projectDir); err == nil {
		return res, nil
	} else {
		// Log but don't fail — RI-10 fallback.
		fmt.Fprintf(os.Stderr,
			"[cks-mcp] ckg: packages.Load failed (%v); falling back to AST-only mode (RI-10)\n", err)
	}
	return extractASTOnly(projectDir)
}

// --- Tier 1: typed (golang.org/x/tools/go/packages) ---

func extractTyped(ctx context.Context, projectDir string) (*ExtractResult, error) {
	cfg := &packages.Config{
		Context: ctx,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports |
			packages.NeedDeps,
		Dir:   projectDir,
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, err
	}
	// packages.Load reports errors via pkg.Errors; bail out if any package
	// failed entirely so we don't silently produce stale data.
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("package %s: %s", pkg.PkgPath, pkg.Errors[0])
		}
	}

	res := &ExtractResult{Mode: ModeTyped}
	nodesByQName := map[string]string{} // qualifiedName → nodeID
	now := time.Now().UTC()

	// First pass: declare every top-level node so edges can refer to them.
	for _, pkg := range pkgs {
		for fi, file := range pkg.Syntax {
			rel := pickFilePath(pkg, fi, projectDir)
			ns := nodesFromFile(pkg.Fset, file, rel, pkg.Name, now)
			for _, n := range ns {
				res.Nodes = append(res.Nodes, n)
				nodesByQName[n.QualifiedName] = n.ID
			}
		}
	}

	// Second pass: extract edges using full type info.
	for _, pkg := range pkgs {
		for fi, file := range pkg.Syntax {
			rel := pickFilePath(pkg, fi, projectDir)
			edges := edgesFromFileTyped(pkg.Fset, file, pkg.TypesInfo, pkg.Name,
				rel, nodesByQName)
			res.Edges = append(res.Edges, edges...)
		}
		// implements: walk all named types vs interfaces in this pkg.
		res.Edges = append(res.Edges, edgesImplementsForPkg(pkg, nodesByQName)...)
	}

	return res, nil
}

// --- Tier 2: AST-only fallback (RI-10) ---

func extractASTOnly(projectDir string) (*ExtractResult, error) {
	res := &ExtractResult{Mode: ModeASTOnly}
	now := time.Now().UTC()
	nodesByQName := map[string]string{}
	// Index of methodSet per type name: typeName → set of method names.
	typeMethods := map[string]map[string]struct{}{}
	interfaceMethods := map[string][]string{} // ifaceQname → method names

	err := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == projectDir {
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.Contains(path, "vendor/") || strings.HasSuffix(path, "_gen.go") {
			return nil
		}
		fset := token.NewFileSet()
		src, rerr := os.ReadFile(path) //nolint:gosec
		if rerr != nil {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, src, parser.ParseComments)
		if perr != nil {
			return nil
		}
		rel := relPath(projectDir, path)
		pkgName := ""
		if file.Name != nil {
			pkgName = file.Name.Name
		}
		ns := nodesFromFile(fset, file, rel, pkgName, now)
		for _, n := range ns {
			res.Nodes = append(res.Nodes, n)
			nodesByQName[n.QualifiedName] = n.ID
		}
		edges, methodMap, ifaceMap := edgesFromFileASTOnly(fset, file, pkgName, rel, nodesByQName)
		res.Edges = append(res.Edges, edges...)
		mergeStringSetMap(typeMethods, methodMap)
		for k, v := range ifaceMap {
			interfaceMethods[k] = append(interfaceMethods[k], v...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Textual implements heuristic: a struct/named type "implements" an
	// interface if its method set is a superset of the interface's methods.
	for ifaceQName, ifaceMethodList := range interfaceMethods {
		ifaceID := nodesByQName[ifaceQName]
		if ifaceID == "" {
			continue
		}
		for typeQName, mset := range typeMethods {
			if typeQName == ifaceQName {
				continue
			}
			if isSuperset(mset, ifaceMethodList) {
				typeID := nodesByQName[typeQName]
				if typeID == "" {
					continue
				}
				res.Edges = append(res.Edges, internaltypes.GraphEdge{
					FromNode:     typeID,
					ToNode:       ifaceID,
					RelationType: internaltypes.RelImplements,
					Confidence:   internaltypes.ConfidenceLow,
				})
			}
		}
	}

	return res, nil
}

func isSuperset(have map[string]struct{}, need []string) bool {
	if len(need) == 0 {
		return false
	}
	for _, m := range need {
		if _, ok := have[m]; !ok {
			return false
		}
	}
	return true
}

// --- Node extraction ---

// nodesFromFile reads top-level declarations into GraphNodes.
func nodesFromFile(
	fset *token.FileSet,
	file *ast.File,
	relPath, pkgName string,
	now time.Time,
) []internaltypes.GraphNode {
	var out []internaltypes.GraphNode
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name == nil {
				continue
			}
			recv, isMethod := receiverTypeName(d)
			qname := pkgName + "." + d.Name.Name
			st := internaltypes.SymbolFunction
			if isMethod {
				qname = fmt.Sprintf("%s.(%s).%s", pkgName, recv, d.Name.Name)
				st = internaltypes.SymbolMethod
			}
			start := fset.Position(d.Pos()).Line
			end := fset.Position(d.End()).Line
			sig := buildSignature(fset, d)
			out = append(out, internaltypes.GraphNode{
				ID:            graphNodeID(qname),
				FilePath:      relPath,
				PackageName:   pkgName,
				SymbolName:    d.Name.Name,
				SymbolType:    st,
				QualifiedName: qname,
				Signature:     sig,
				StartLine:     start,
				EndLine:       end,
				IndexedAt:     now,
			})
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || ts.Name == nil {
					continue
				}
				qname := pkgName + "." + ts.Name.Name
				st := internaltypes.SymbolStruct
				if _, isIface := ts.Type.(*ast.InterfaceType); isIface {
					st = internaltypes.SymbolInterface
				}
				start := fset.Position(ts.Pos()).Line
				end := fset.Position(ts.End()).Line
				out = append(out, internaltypes.GraphNode{
					ID:            graphNodeID(qname),
					FilePath:      relPath,
					PackageName:   pkgName,
					SymbolName:    ts.Name.Name,
					SymbolType:    st,
					QualifiedName: qname,
					Signature:     "type " + ts.Name.Name,
					StartLine:     start,
					EndLine:       end,
					IndexedAt:     now,
				})
			}
		}
	}
	return out
}

// --- Edge extraction (typed) ---

func edgesFromFileTyped(
	fset *token.FileSet,
	file *ast.File,
	info *types.Info,
	pkgName, relPath string,
	nodesByQName map[string]string,
) []internaltypes.GraphEdge {
	var edges []internaltypes.GraphEdge

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		recv, isMethod := receiverTypeName(fn)
		fromQName := pkgName + "." + fn.Name.Name
		if isMethod {
			fromQName = fmt.Sprintf("%s.(%s).%s", pkgName, recv, fn.Name.Name)
		}
		fromID := nodesByQName[fromQName]
		if fromID == "" {
			continue
		}
		seen := map[string]struct{}{}

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				if e := resolveCallEdge(info, fromID, x, pkgName, nodesByQName); e != nil {
					key := edgeKey(*e)
					if _, dup := seen[key]; !dup {
						seen[key] = struct{}{}
						edges = append(edges, *e)
					}
				}
			case *ast.SelectorExpr:
				// Field reads/writes via X.Y are detected by walking AssignStmt parents.
			case *ast.SendStmt:
				edges = append(edges, chanEdge(info, fromID, x.Chan, "send"))
			case *ast.UnaryExpr:
				if x.Op == token.ARROW {
					edges = append(edges, chanEdge(info, fromID, x.X, "receive"))
				}
			}
			return true
		})

		// Detect field read/write via assignments.
		edges = append(edges, detectFieldAccess(info, fn, fromID)...)
		// Detect uses_type via Params/Returns/Var types.
		edges = append(edges, detectUsesType(info, fn, fromID, nodesByQName, pkgName)...)
	}

	// Embeds: scan struct type decls.
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name == nil {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			fromQName := pkgName + "." + ts.Name.Name
			fromID := nodesByQName[fromQName]
			if fromID == "" {
				continue
			}
			for _, f := range st.Fields.List {
				if len(f.Names) != 0 {
					continue // not anonymous → not embedded
				}
				typeName := exprTypeName(f.Type)
				if typeName == "" {
					continue
				}
				if toID, ok := lookupTypeNode(nodesByQName, pkgName, typeName); ok {
					edges = append(edges, internaltypes.GraphEdge{
						FromNode:     fromID,
						ToNode:       toID,
						RelationType: internaltypes.RelEmbeds,
						Confidence:   internaltypes.ConfidenceHigh,
					})
				}
			}
		}
	}
	return edges
}

// edgesImplementsForPkg uses go/types to compute structural implements pairs.
func edgesImplementsForPkg(pkg *packages.Package, nodesByQName map[string]string) []internaltypes.GraphEdge {
	if pkg.Types == nil {
		return nil
	}
	var edges []internaltypes.GraphEdge
	scope := pkg.Types.Scope()
	names := scope.Names()

	// Split into interfaces and named types.
	var ifaceObjs, namedObjs []types.Object
	for _, name := range names {
		obj := scope.Lookup(name)
		tn, ok := obj.(*types.TypeName)
		if !ok || tn.Type() == nil {
			continue
		}
		under := tn.Type().Underlying()
		if _, ok := under.(*types.Interface); ok {
			ifaceObjs = append(ifaceObjs, obj)
		} else {
			namedObjs = append(namedObjs, obj)
		}
	}

	for _, iobj := range ifaceObjs {
		ifaceType := iobj.Type().Underlying().(*types.Interface)
		if ifaceType.NumMethods() == 0 {
			continue // empty interface; would match everything
		}
		ifaceQName := pkg.Name + "." + iobj.Name()
		ifaceID := nodesByQName[ifaceQName]
		if ifaceID == "" {
			continue
		}
		for _, nobj := range namedObjs {
			nt := nobj.Type()
			if types.Implements(nt, ifaceType) || types.Implements(types.NewPointer(nt), ifaceType) {
				toQName := pkg.Name + "." + nobj.Name()
				fromID := nodesByQName[toQName]
				if fromID == "" || fromID == ifaceID {
					continue
				}
				edges = append(edges, internaltypes.GraphEdge{
					FromNode:     fromID,
					ToNode:       ifaceID,
					RelationType: internaltypes.RelImplements,
					Confidence:   internaltypes.ConfidenceHigh,
				})
			}
		}
	}
	return edges
}

// --- AST-only edges ---

// edgesFromFileASTOnly returns name-based call edges, struct embedding,
// and the per-type method set + per-interface method names used by the
// structural-implements heuristic.
func edgesFromFileASTOnly(
	fset *token.FileSet,
	file *ast.File,
	pkgName, relPath string,
	nodesByQName map[string]string,
) (
	edges []internaltypes.GraphEdge,
	typeMethods map[string]map[string]struct{},
	interfaceMethods map[string][]string,
) {
	typeMethods = map[string]map[string]struct{}{}
	interfaceMethods = map[string][]string{}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name == nil {
				continue
			}
			recv, isMethod := receiverTypeName(d)
			fromQName := pkgName + "." + d.Name.Name
			if isMethod {
				fromQName = fmt.Sprintf("%s.(%s).%s", pkgName, recv, d.Name.Name)
				// register method on type
				key := pkgName + "." + recv
				if typeMethods[key] == nil {
					typeMethods[key] = map[string]struct{}{}
				}
				typeMethods[key][d.Name.Name] = struct{}{}
			}
			fromID := nodesByQName[fromQName]
			if fromID == "" || d.Body == nil {
				continue
			}
			seen := map[string]struct{}{}
			ast.Inspect(d.Body, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					name := callTargetName(c.Fun)
					if name == "" {
						return true
					}
					if toID, ok2 := lookupCallNode(nodesByQName, pkgName, name); ok2 {
						k := fromID + "|" + toID + "|calls"
						if _, dup := seen[k]; !dup {
							seen[k] = struct{}{}
							edges = append(edges, internaltypes.GraphEdge{
								FromNode:     fromID,
								ToNode:       toID,
								RelationType: internaltypes.RelCalls,
								Confidence:   internaltypes.ConfidenceLow,
							})
						}
					}
				}
				return true
			})
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				switch tt := ts.Type.(type) {
				case *ast.StructType:
					fromQName := pkgName + "." + ts.Name.Name
					fromID := nodesByQName[fromQName]
					for _, f := range tt.Fields.List {
						if len(f.Names) != 0 {
							continue
						}
						typeName := exprTypeName(f.Type)
						if typeName == "" {
							continue
						}
						if toID, ok2 := lookupTypeNode(nodesByQName, pkgName, typeName); ok2 && fromID != "" {
							edges = append(edges, internaltypes.GraphEdge{
								FromNode:     fromID,
								ToNode:       toID,
								RelationType: internaltypes.RelEmbeds,
								Confidence:   internaltypes.ConfidenceMedium,
							})
						}
					}
				case *ast.InterfaceType:
					ifaceQName := pkgName + "." + ts.Name.Name
					for _, m := range tt.Methods.List {
						for _, name := range m.Names {
							interfaceMethods[ifaceQName] = append(interfaceMethods[ifaceQName], name.Name)
						}
					}
				}
			}
		}
	}
	_ = relPath
	_ = fset
	return
}

// --- Field / Type / Channel helpers ---

func detectFieldAccess(info *types.Info, fn *ast.FuncDecl, fromID string) []internaltypes.GraphEdge {
	var edges []internaltypes.GraphEdge
	if fn.Body == nil || info == nil {
		return nil
	}
	for _, stmt := range fn.Body.List {
		switch as := stmt.(type) {
		case *ast.AssignStmt:
			for _, lhs := range as.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok {
					if id := fieldOwnerID(info, sel); id != "" {
						edges = append(edges, internaltypes.GraphEdge{
							FromNode:     fromID,
							ToNode:       id,
							RelationType: internaltypes.RelWritesField,
							Confidence:   internaltypes.ConfidenceMedium,
							Metadata:     map[string]any{"field": sel.Sel.Name},
						})
					}
				}
			}
			for _, rhs := range as.Rhs {
				if sel, ok := rhs.(*ast.SelectorExpr); ok {
					if id := fieldOwnerID(info, sel); id != "" {
						edges = append(edges, internaltypes.GraphEdge{
							FromNode:     fromID,
							ToNode:       id,
							RelationType: internaltypes.RelReadsField,
							Confidence:   internaltypes.ConfidenceMedium,
							Metadata:     map[string]any{"field": sel.Sel.Name},
						})
					}
				}
			}
		}
	}
	return edges
}

func detectUsesType(
	info *types.Info, fn *ast.FuncDecl, fromID string,
	nodesByQName map[string]string, pkgName string,
) []internaltypes.GraphEdge {
	var edges []internaltypes.GraphEdge
	add := func(typ ast.Expr) {
		typeName := exprTypeName(typ)
		if typeName == "" {
			return
		}
		if toID, ok := lookupTypeNode(nodesByQName, pkgName, typeName); ok && toID != fromID {
			edges = append(edges, internaltypes.GraphEdge{
				FromNode:     fromID,
				ToNode:       toID,
				RelationType: internaltypes.RelUsesType,
				Confidence:   internaltypes.ConfidenceMedium,
			})
		}
	}
	if fn.Type != nil {
		if fn.Type.Params != nil {
			for _, p := range fn.Type.Params.List {
				add(p.Type)
			}
		}
		if fn.Type.Results != nil {
			for _, r := range fn.Type.Results.List {
				add(r.Type)
			}
		}
	}
	_ = info // reserved for richer type resolution
	return edges
}

func chanEdge(info *types.Info, fromID string, expr ast.Expr, direction string) internaltypes.GraphEdge {
	chType := ""
	if info != nil {
		if tv, ok := info.Types[expr]; ok && tv.Type != nil {
			chType = tv.Type.String()
		}
	}
	if chType == "" {
		chType = exprTypeName(expr)
	}
	return internaltypes.GraphEdge{
		FromNode:     fromID,
		ToNode:       fromID, // self-loop; channels link a function to itself with metadata
		RelationType: internaltypes.RelChannels,
		Confidence:   internaltypes.ConfidenceMedium,
		Metadata: map[string]any{
			"direction":    direction,
			"channel_type": chType,
		},
	}
}

func resolveCallEdge(
	info *types.Info, fromID string, call *ast.CallExpr,
	pkgName string, nodesByQName map[string]string,
) *internaltypes.GraphEdge {
	name := callTargetName(call.Fun)
	if name == "" {
		return nil
	}
	if toID, ok := lookupCallNode(nodesByQName, pkgName, name); ok && toID != "" {
		return &internaltypes.GraphEdge{
			FromNode:     fromID,
			ToNode:       toID,
			RelationType: internaltypes.RelCalls,
			Confidence:   internaltypes.ConfidenceHigh,
		}
	}
	_ = info
	return nil
}

func callTargetName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	}
	return ""
}

func lookupCallNode(nodesByQName map[string]string, pkgName, name string) (string, bool) {
	// Prefer same-package match.
	if id, ok := nodesByQName[pkgName+"."+name]; ok {
		return id, true
	}
	// Otherwise scan any package.
	suffix := "." + name
	for q, id := range nodesByQName {
		if strings.HasSuffix(q, suffix) {
			return id, true
		}
	}
	return "", false
}

func lookupTypeNode(nodesByQName map[string]string, pkgName, name string) (string, bool) {
	if id, ok := nodesByQName[pkgName+"."+name]; ok {
		return id, true
	}
	suffix := "." + name
	for q, id := range nodesByQName {
		if strings.HasSuffix(q, suffix) {
			return id, true
		}
	}
	return "", false
}

func fieldOwnerID(info *types.Info, sel *ast.SelectorExpr) string {
	if info == nil {
		return ""
	}
	tv, ok := info.Types[sel.X]
	if !ok || tv.Type == nil {
		return ""
	}
	t := tv.Type
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj() == nil {
		return ""
	}
	pkg := named.Obj().Pkg()
	if pkg == nil {
		return ""
	}
	qname := pkg.Name() + "." + named.Obj().Name()
	return graphNodeID(qname)
}

// --- Helpers ---

func receiverTypeName(fn *ast.FuncDecl) (string, bool) {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return "", false
	}
	t := fn.Recv.List[0].Type
	for {
		s, ok := t.(*ast.StarExpr)
		if !ok {
			break
		}
		t = s.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name, true
	}
	return "", true
}

func exprTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return exprTypeName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	}
	return ""
}

func buildSignature(fset *token.FileSet, fn *ast.FuncDecl) string {
	var buf bytes.Buffer
	buf.WriteString("func ")
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		buf.WriteString("(")
		_ = printer.Fprint(&buf, fset, fn.Recv.List[0].Type)
		buf.WriteString(") ")
	}
	buf.WriteString(fn.Name.Name)
	if fn.Type != nil {
		var tb bytes.Buffer
		_ = printer.Fprint(&tb, fset, fn.Type)
		buf.WriteString(strings.TrimPrefix(tb.String(), "func"))
	}
	return strings.TrimSpace(strings.ReplaceAll(buf.String(), "\n", " "))
}

func graphNodeID(qname string) string {
	h := sha256.Sum256([]byte(qname))
	return hex.EncodeToString(h[:8])
}

func edgeKey(e internaltypes.GraphEdge) string {
	return e.FromNode + "|" + e.ToNode + "|" + string(e.RelationType)
}

func relPath(base, full string) string {
	r, err := filepath.Rel(base, full)
	if err != nil {
		return full
	}
	return filepath.ToSlash(r)
}

// pickFilePath returns a repo-relative file path for pkg.Syntax[fi].
// packages.Load doesn't always populate CompiledGoFiles in lockstep with
// Syntax (e.g., for synthesized files), so we fall back to the FileSet.
func pickFilePath(pkg *packages.Package, fi int, projectDir string) string {
	if fi < len(pkg.CompiledGoFiles) && pkg.CompiledGoFiles[fi] != "" {
		return relPath(projectDir, pkg.CompiledGoFiles[fi])
	}
	if fi < len(pkg.GoFiles) && pkg.GoFiles[fi] != "" {
		return relPath(projectDir, pkg.GoFiles[fi])
	}
	if pkg.Fset != nil && fi < len(pkg.Syntax) {
		pos := pkg.Fset.Position(pkg.Syntax[fi].Pos())
		if pos.Filename != "" {
			return relPath(projectDir, pos.Filename)
		}
	}
	return ""
}

func mergeStringSetMap(dst, src map[string]map[string]struct{}) {
	for k, v := range src {
		if dst[k] == nil {
			dst[k] = map[string]struct{}{}
		}
		for m := range v {
			dst[k][m] = struct{}{}
		}
	}
}
