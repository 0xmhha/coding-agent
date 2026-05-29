package ckg

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	internaltypes "github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

// AnalyzeConcurrency walks each FuncDecl in the project AST set and produces
// a ConcurrencyContext per function. The analyzer is intentionally
// best-effort: interface-dispatch goroutines and reflect-based dispatch
// are recorded with `confidence = "unknown"` per RI-11.
//
// fileSet is the file→AST map keyed by repo-relative file paths so the caller
// can run extraction in a single pass.
type FileAST struct {
	FileSet  *token.FileSet
	File     *ast.File
	RelPath  string
	PkgName  string
}

// AnalyzeConcurrency returns one ConcurrencyContext per FuncDecl in files.
// nodesByQName lets us map qualified names back to node IDs.
func AnalyzeConcurrency(
	files []FileAST,
	nodesByQName map[string]string,
) []internaltypes.ConcurrencyContext {
	var out []internaltypes.ConcurrencyContext

	// First pass: collect per-function `go` launch edges so we can also
	// populate GoroutineContext.LaunchedBy in a second pass.
	launches := map[string][]string{} // launcherFn → []launchedFn

	for _, f := range files {
		for _, decl := range f.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			fromQName := qualifiedFuncName(f.PkgName, fn)
			fromID := nodesByQName[fromQName]
			if fromID == "" {
				continue
			}
			cc := analyzeFunc(fn, f.PkgName, fromID, nodesByQName, launches)
			out = append(out, cc)
		}
	}

	// Second pass: assemble LaunchedBy.
	launchedBy := map[string][]string{}
	for launcher, launchedFns := range launches {
		for _, lf := range launchedFns {
			launchedBy[lf] = append(launchedBy[lf], launcher)
		}
	}
	for i := range out {
		if extras, ok := launchedBy[out[i].NodeID]; ok {
			out[i].GoroutineContext.LaunchedBy = uniqueStrings(append(out[i].GoroutineContext.LaunchedBy, extras...))
		}
	}
	return out
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// analyzeFunc inspects one function body and returns its ConcurrencyContext.
func analyzeFunc(
	fn *ast.FuncDecl,
	pkgName, fromID string,
	nodesByQName map[string]string,
	launches map[string][]string,
) internaltypes.ConcurrencyContext {
	cc := internaltypes.ConcurrencyContext{
		NodeID:     fromID,
		Confidence: internaltypes.ConfidenceMedium,
		Risk:       internaltypes.RiskAssessment{RaceConditionRisk: "none"},
	}

	syncSet := map[string]struct{}{}
	chanOps := []internaltypes.ChannelOperation{}
	confidenceDowngraded := false
	unprotectedAccess := []string{}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {

		case *ast.GoStmt:
			target := callTargetName(x.Call.Fun)
			confidence := internaltypes.ConfidenceMedium
			if isInterfaceDispatch(x.Call.Fun) {
				// RI-11: interface-dispatch goroutines are not statically
				// resolvable; mark the overall function as unknown.
				confidenceDowngraded = true
			}
			if target == "" {
				return true
			}
			if launchedID, ok := lookupCallNode(nodesByQName, pkgName, target); ok && launchedID != "" {
				launches[fromID] = append(launches[fromID], launchedID)
				cc.GoroutineContext.Launches = append(cc.GoroutineContext.Launches, launchedID)
			}
			_ = confidence

		case *ast.SendStmt:
			chanOps = append(chanOps, internaltypes.ChannelOperation{
				Channel:   exprText(x.Chan),
				Direction: "send",
			})

		case *ast.UnaryExpr:
			if x.Op == token.ARROW {
				chanOps = append(chanOps, internaltypes.ChannelOperation{
					Channel:   exprText(x.X),
					Direction: "receive",
				})
			}

		case *ast.CallExpr:
			method := callTargetName(x.Fun)
			variable := receiverExpr(x.Fun)
			switch method {
			case "Lock", "Unlock", "RLock", "RUnlock":
				syncSet["mutex:"+variable] = struct{}{}
			case "Wait", "Add", "Done":
				if isWaitGroupCall(method, variable) {
					syncSet["waitgroup:"+variable] = struct{}{}
				}
			case "LoadInt32", "LoadInt64", "StoreInt32", "StoreInt64",
				"AddInt32", "AddInt64", "CompareAndSwapInt32", "CompareAndSwapInt64":
				syncSet["atomic:"+variable] = struct{}{}
			}

		case *ast.SelectStmt:
			syncSet["select:_"] = struct{}{}

		case *ast.AssignStmt:
			// Coarse shared-write detection: writes to receiver fields.
			for _, lhs := range x.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok {
					if isReceiverFieldAccess(fn, sel) {
						unprotectedAccess = append(unprotectedAccess,
							exprText(sel.X)+"."+sel.Sel.Name)
					}
				}
			}
		}
		return true
	})

	// Materialize syncMechanisms.
	for key := range syncSet {
		parts := strings.SplitN(key, ":", 2)
		var sm internaltypes.SyncMechanism
		sm.Type = parts[0]
		if len(parts) == 2 {
			sm.Variable = parts[1]
		}
		sm.Scope = "partial"
		cc.SyncMechanisms = append(cc.SyncMechanisms, sm)
	}
	cc.ChannelOperations = chanOps

	// Risk assessment: drop confidence + classify.
	if confidenceDowngraded {
		cc.Confidence = internaltypes.ConfidenceUnknown
		cc.Risk.RaceConditionRisk = "unknown"
		cc.Risk.Note = "interface-dispatch goroutine detected; static analysis cannot trace targets (RI-11)"
	} else if len(unprotectedAccess) > 0 && !hasMutexLikeSync(cc.SyncMechanisms) {
		cc.Risk.RaceConditionRisk = "high"
		cc.Risk.UnprotectedSharedAccess = unprotectedAccess
		cc.Risk.Note = "writes to receiver state without mutex/atomic protection"
	} else if len(unprotectedAccess) > 0 {
		cc.Risk.RaceConditionRisk = "low"
		cc.Risk.UnprotectedSharedAccess = unprotectedAccess
	}

	return cc
}

func hasMutexLikeSync(sm []internaltypes.SyncMechanism) bool {
	for _, m := range sm {
		switch m.Type {
		case "mutex", "rwmutex", "atomic", "select":
			return true
		}
	}
	return false
}

func isReceiverFieldAccess(fn *ast.FuncDecl, sel *ast.SelectorExpr) bool {
	if fn.Recv == nil || len(fn.Recv.List) == 0 || len(fn.Recv.List[0].Names) == 0 {
		return false
	}
	recvName := fn.Recv.List[0].Names[0].Name
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == recvName
}

// isInterfaceDispatch returns true if the call expression looks like it
// targets an interface method (e.g., `iface.Method(...)` where iface has
// no concrete owner we can resolve at extraction time).
func isInterfaceDispatch(fun ast.Expr) bool {
	// Heuristic: SelectorExpr on a non-package identifier with a single-word
	// receiver that doesn't look like a package import alias is the common
	// shape of interface dispatch in Go.
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	// Single uppercase letter — common interface variable convention.
	if len(id.Name) == 1 {
		return true
	}
	return false
}

func receiverExpr(fun ast.Expr) string {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	return exprText(sel.X)
}

func isWaitGroupCall(method, variable string) bool {
	// Cheap heuristic: variable contains "wg" or "WG".
	if variable == "" {
		return false
	}
	lv := strings.ToLower(variable)
	if strings.Contains(lv, "wg") {
		return true
	}
	// Methods like Wait/Add/Done are also used by other types, so without
	// type info we err on the side of false to avoid noise.
	_ = method
	return false
}

// qualifiedFuncName mirrors the formatting used in nodesFromFile so the
// concurrency analyzer can look up the node id.
func qualifiedFuncName(pkgName string, fn *ast.FuncDecl) string {
	if fn.Name == nil {
		return ""
	}
	recv, isMethod := receiverTypeName(fn)
	if isMethod {
		return fmt.Sprintf("%s.(%s).%s", pkgName, recv, fn.Name.Name)
	}
	return pkgName + "." + fn.Name.Name
}

// exprText returns a short textual form of an expression (e.g., for channel
// or selector display). For names it returns the identifier; for selector
// X.Y it returns X. We deliberately avoid the printer here to stay cheap.
func exprText(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return exprText(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return exprText(t.X)
	}
	return "_"
}
