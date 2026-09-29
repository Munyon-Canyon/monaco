package nogo

import (
	"go/ast"
	"go/types"
	"path/filepath"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

func TestMainAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "testmain",
		Doc:  "reports a TestMain outside main_test.go or whose body is not exactly testkit.Main(m, opts...)",
		Run:  runTestMain,
	}
}

func runTestMain(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name.Name != "TestMain" {
				continue
			}
			if filepath.Base(pass.Fset.File(fn.Pos()).Name()) != "main_test.go" {
				pass.Reportf(fn.Pos(), "TestMain belongs in main_test.go, one per package")
			}
			if !callsTestkitMain(pass, fn) {
				pass.Reportf(fn.Pos(), "TestMain body must be exactly testkit.Main(m, opts...)")
			}
		}
	}
	return nil, nil
}

func callsTestkitMain(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	if len(fn.Body.List) != 1 {
		return false
	}
	stmt, ok := fn.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := stmt.X.(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return false
	}
	callee, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
	if !ok || callee.Pkg() == nil || callee.Pkg().Path() != modulePath+"/internal/testkit" || callee.Name() != "Main" {
		return false
	}
	arg, ok := call.Args[0].(*ast.Ident)
	param := fn.Type.Params.List[0]
	return ok && len(param.Names) == 1 && pass.TypesInfo.Uses[arg] == pass.TypesInfo.Defs[param.Names[0]]
}
