package nogo

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestRunUowdo_readsTheEmbeddedAllowList(t *testing.T) {
	t.Parallel()
	orig := uowdoAllow
	t.Cleanup(func() { uowdoAllow = orig })
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "/repo/internal/modules/m/m.go", "package m\n\nvar _ = f()\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Analyzer: UowdoAnalyzer()}

	uowdoAllow = "onlypath\n"
	if _, err := runUowdo(pass); err == nil || !strings.Contains(err.Error(), "allow line 1") {
		t.Fatalf("runUowdo = %v, want the bad allow file", err)
	}

	uowdoAllow = "internal/modules/m/m.go a reason\n"
	if _, err := runUowdo(pass); err != nil {
		t.Fatalf("runUowdo = %v, want the allow-listed file skipped", err)
	}
}
