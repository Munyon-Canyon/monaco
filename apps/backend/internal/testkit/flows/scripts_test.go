package flows_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestScripts_registersEveryFlowScriptInThePackage(t *testing.T) {
	t.Parallel()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	script := regexp.MustCompile(`^F[0-9]+[A-Z]`)
	var declared []string
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && script.MatchString(fn.Name.Name) {
				declared = append(declared, fn.Name.Name)
			}
		}
	}
	registered := slices.Sorted(maps.Keys(flows.Scripts()))
	if slices.Sort(declared); !slices.Equal(declared, registered) {
		t.Fatalf("flow scripts declared %v, registered in Scripts() %v", declared, registered)
	}
}
