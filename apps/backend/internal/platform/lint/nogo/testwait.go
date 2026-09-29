package nogo

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

const testwaitFix = "wait on a signal, use synctest.Test, testkit.Eventually or testkit.AssertNoRedelivery"

func TestwaitAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "testwait",
		Doc: "reports fixed <-time.After waits and time.After, time.Tick or time.NewTicker poll loops in tests " +
			"outside testing/synctest; testkit is exempt because it owns the wait helpers",
		Run: runTestwait,
	}
}

func runTestwait(pass *analysis.Pass) (any, error) {
	testkit := modulePath + "/internal/testkit"
	pkg := strings.TrimSuffix(pass.Pkg.Path(), "_test")
	if pkg == testkit || strings.HasPrefix(pkg, testkit+"/") {
		return nil, nil
	}
	for _, file := range pass.Files {
		if strings.HasSuffix(pass.Fset.File(file.Pos()).Name(), "_test.go") {
			reportTestwaits(pass, file)
		}
	}
	return nil, nil
}

func reportTestwaits(pass *analysis.Pass, file *ast.File) {
	parents := parentsOf(file)
	bubbles := synctestBubbles(pass, file)
	fixed := map[*ast.CallExpr]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return !bubbles[n]
		case *ast.ExprStmt:
			if call := fixedWait(pass, parents, n); call != nil {
				fixed[call] = true
				pass.Reportf(n.Pos(), "fixed wait <-time.After: %s", testwaitFix)
			}
		case *ast.CallExpr:
			if name := timerName(pass, n); name != "" && !fixed[n] && insideLoop(parents, n) {
				pass.Reportf(n.Pos(), "poll loop time.%s in a for loop: %s", name, testwaitFix)
			}
		}
		return true
	})
}

func fixedWait(pass *analysis.Pass, parents map[ast.Node]ast.Node, stmt *ast.ExprStmt) *ast.CallExpr {
	if _, isCase := parents[stmt].(*ast.CommClause); isCase {
		return nil
	}
	recv, ok := stmt.X.(*ast.UnaryExpr)
	if !ok || recv.Op != token.ARROW {
		return nil
	}
	call, ok := recv.X.(*ast.CallExpr)
	if !ok || timerName(pass, call) != "After" {
		return nil
	}
	return call
}

func timerName(pass *analysis.Pass, call *ast.CallExpr) string {
	fn, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "time" || fn.Type().(*types.Signature).Recv() != nil {
		return ""
	}
	switch fn.Name() {
	case "After", "Tick", "NewTicker":
		return fn.Name()
	default:
		return ""
	}
}

func insideLoop(parents map[ast.Node]ast.Node, n ast.Node) bool {
	for p := parents[n]; p != nil; p = parents[p] {
		switch p.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			return true
		case *ast.FuncLit, *ast.FuncDecl:
			return false
		}
	}
	return false
}
