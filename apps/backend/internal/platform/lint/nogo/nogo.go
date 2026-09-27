package nogo

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
)

const modulePath = "github.com/monaco/monaco/apps/backend"

func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "nogo",
		Doc:  "reports go statements outside internal/platform and cmd",
		Run:  run,
	}
}

func run(pass *analysis.Pass) (any, error) {
	if mayStartGoroutines(pass.Pkg.Path()) {
		return nil, nil
	}
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			if stmt, ok := n.(*ast.GoStmt); ok {
				pass.Reportf(stmt.Pos(), "bare go statement: use platform/concurrency or errgroup")
			}
			return true
		})
	}
	return nil, nil
}

func mayStartGoroutines(pkgPath string) bool {
	return strings.HasPrefix(pkgPath, modulePath+"/cmd/") ||
		strings.HasPrefix(pkgPath, modulePath+"/internal/platform/")
}
