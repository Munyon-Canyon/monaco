package nogo

import (
	_ "embed"
	"go/ast"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

//go:embed uowdo.allow
var uowdoAllow string

const (
	chainPkg       = modulePath + "/internal/platform/chain"
	httpclientPkg  = modulePath + "/internal/platform/httpclient"
	busPkg         = modulePath + "/internal/platform/bus"
	dbPkg          = modulePath + "/internal/platform/db"
	observabilityP = modulePath + "/internal/platform/observability"
)

func UowdoAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "uowdo",
		Doc:  "reports outbound chain, relayer and HTTP calls, Info-level logs and Publish calls not wrapped in AfterCommit inside a db.UnitOfWork.Do closure",
		Run:  runUowdo,
	}
}

func runUowdo(pass *analysis.Pass) (any, error) {
	allowed, err := allowFrom(uowdoAllow)
	if err != nil {
		return nil, err
	}
	local := localFuncs(pass)
	for _, file := range pass.Files {
		if allowedPath(allowed, pass.Fset.File(file.Pos()).Name()) {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isMethod(pass, call, "(*"+dbPkg+".UnitOfWork).Do") {
				return true
			}
			if lit, ok := call.Args[len(call.Args)-1].(*ast.FuncLit); ok {
				reportInsideTx(pass, lit.Body, local)
			}
			return true
		})
	}
	return nil, nil
}

func isMethod(pass *analysis.Pass, call *ast.CallExpr, fullName string) bool {
	fn := typeutil.StaticCallee(pass.TypesInfo, call)
	return fn != nil && fn.FullName() == fullName
}

func localFuncs(pass *analysis.Pass) map[*types.Func]*ast.FuncDecl {
	local := map[*types.Func]*ast.FuncDecl{}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			if d, ok := decl.(*ast.FuncDecl); ok && d.Body != nil {
				if fn, ok := pass.TypesInfo.Defs[d.Name].(*types.Func); ok {
					local[fn] = d
				}
			}
		}
	}
	return local
}

func reportInsideTx(pass *analysis.Pass, body *ast.BlockStmt, local map[*types.Func]*ast.FuncDecl) {
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isMethod(pass, call, "("+dbPkg+".Tx).AfterCommit") {
			return false
		}
		if msg := uowdoViolation(pass, call); msg != "" {
			pass.Reportf(call.Pos(), "%s inside uow.Do: %s", types.ExprString(call.Fun), msg)
			return true
		}
		if helper := typeutil.StaticCallee(pass.TypesInfo, call); local[helper] != nil {
			reportVia(pass, call, helper, local)
		}
		return true
	})
}

func reportVia(pass *analysis.Pass, site *ast.CallExpr, helper *types.Func, local map[*types.Func]*ast.FuncDecl) {
	seen := map[*types.Func]bool{}
	var walk func(fn *types.Func)
	walk = func(fn *types.Func) {
		seen[fn] = true
		ast.Inspect(local[fn].Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if isMethod(pass, call, "("+dbPkg+".Tx).AfterCommit") {
				return false
			}
			if msg := uowdoViolation(pass, call); msg != "" {
				pass.Reportf(
					site.Pos(),
					"%s inside uow.Do: %s (via %s)",
					types.ExprString(call.Fun),
					msg,
					helper.Name(),
				)
			} else if next := typeutil.StaticCallee(pass.TypesInfo, call); local[next] != nil && !seen[next] {
				walk(next)
			}
			return true
		})
	}
	walk(helper)
}

func uowdoViolation(pass *analysis.Pass, call *ast.CallExpr) string {
	fn, dynamic := calleeOf(pass, call)
	if fn == nil || fn.Pkg() == nil || dynamic && fn.Pkg().Path() != pass.Pkg.Path() {
		return ""
	}
	path, name := fn.Pkg().Path(), fn.Name()
	switch {
	case isOutboundChain(pass, fn, dynamic):
		return "outbound chain or relayer call; run it before or after the transaction"
	case path == httpclientPkg || strings.HasPrefix(fn.FullName(), "(*net/http.Client)."):
		return "HTTP call; run it before or after the transaction"
	case path == busPkg && (name == "Publish" || name == "PublishHint"):
		return "publish not wrapped in tx.AfterCommit; append the event with tx.Events or publish after commit"
	case isInfoLog(path, name):
		return "log at Info or above; log at Debug, or log after the commit"
	}
	return ""
}

func calleeOf(pass *analysis.Pass, call *ast.CallExpr) (fn *types.Func, dynamic bool) {
	if fn = typeutil.StaticCallee(pass.TypesInfo, call); fn != nil {
		return fn, false
	}
	fn, _ = typeutil.Callee(pass.TypesInfo, call).(*types.Func)
	return fn, fn != nil
}

func isOutboundChain(pass *analysis.Pass, fn *types.Func, dynamic bool) bool {
	path := fn.Pkg().Path()
	inChain := path == chainPkg || strings.HasPrefix(path, chainPkg+"/")
	return inChain && (dynamic || path != pass.Pkg.Path()) && !runsInTx(fn) && takesContext(fn)
}

func takesContext(fn *types.Func) bool {
	params := fn.Type().(*types.Signature).Params()
	for i := range params.Len() {
		if types.TypeString(params.At(i).Type(), nil) == "context.Context" {
			return true
		}
	}
	return false
}

func isInfoLog(path, name string) bool {
	switch path {
	case "log/slog":
		return slices.Contains([]string{"Info", "Warn", "Error", "InfoContext", "WarnContext", "ErrorContext"}, name)
	case observabilityP:
		return slices.Contains([]string{"Info", "Degraded", "Alert"}, name)
	case observabilityP + "/boundary":
		return slices.Contains([]string{"Warn", "Error"}, name)
	}
	return false
}

func runsInTx(fn *types.Func) bool {
	if strings.HasSuffix(fn.Pkg().Path(), "/sqlc") {
		return true
	}
	params := fn.Type().(*types.Signature).Params()
	for i := range params.Len() {
		if named, ok := params.At(i).Type().(*types.Named); ok && named.Obj().Name() == "DBTX" {
			return true
		}
	}
	return false
}
