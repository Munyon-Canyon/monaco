package faultpoint_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

const importPath = "github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"

func registered() map[string]faultpoint.Name {
	return map[string]faultpoint.Name{
		"AfterCreate":  faultpoint.AfterCreate,
		"AfterSign":    faultpoint.AfterSign,
		"AfterExecute": faultpoint.AfterExecute,
		"BeforeCommit": faultpoint.BeforeCommit,
		"AfterPublish": faultpoint.AfterPublish,
	}
}

type scan struct {
	sites []string
	bad   []string
}

func scanCallSites(root string) (scan, error) {
	var s scan
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != root && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return s.file(path, filepath.ToSlash(rel))
	})
	return s, err
}

func (s *scan) file(path, rel string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return fmt.Errorf("parse %s: %w", rel, err)
	}
	alias := importedAs(f)
	if alias == "" {
		return nil
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isQualified(call.Fun, alias, "Hit", "Armed") || len(call.Args) != 2 {
			return true
		}
		at := fmt.Sprintf("%s:%d", rel, fset.Position(call.Pos()).Line)
		if isQualified(call.Args[1], alias, registeredIdents()...) {
			s.sites = append(s.sites, at)
		} else {
			s.bad = append(s.bad, at)
		}
		return true
	})
	return nil
}

func importedAs(f *ast.File) string {
	for _, imp := range f.Imports {
		if p, _ := strconv.Unquote(imp.Path.Value); p != importPath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return "faultpoint"
	}
	return ""
}

func isQualified(e ast.Expr, pkg string, names ...string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == pkg && slices.Contains(names, sel.Sel.Name)
}

func registeredIdents() []string {
	out := make([]string, 0, len(registered()))
	for ident := range registered() {
		out = append(out, ident)
	}
	return out
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test directory")
		}
		dir = parent
	}
}

func TestRegistered_coversEveryName(t *testing.T) {
	t.Parallel()
	values := make([]faultpoint.Name, 0, len(registered()))
	for _, n := range registered() {
		values = append(values, n)
	}
	slices.Sort(values)
	if !slices.Equal(values, faultpoint.Names()) {
		t.Fatalf("registered constants %v, Names() %v", values, faultpoint.Names())
	}
}

func TestHit_everyCallSiteNamesARegisteredConstant(t *testing.T) {
	t.Parallel()
	s, err := scanCallSites(moduleRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.bad) > 0 {
		t.Fatalf("faultpoint calls without a registered faultpoint.<Const> name: %v", s.bad)
	}
	for _, want := range []string{"internal/platform/db/uow.go:", "internal/platform/bus/relay.go:"} {
		if !slices.ContainsFunc(s.sites, func(site string) bool { return strings.HasPrefix(site, want) }) {
			t.Errorf("no faultpoint.Hit found in %s; sites = %v", want, s.sites)
		}
	}
}

func TestScanCallSites_flagsLiteralsUnknownConstantsAndVariables(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(rel, src string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("p/p.go", `package p

import (
	"context"

	fp "`+importPath+`"
)

func f(ctx context.Context, name fp.Name) {
	fp.Hit(ctx, fp.BeforeCommit)
	fp.Hit(ctx, "before-commit")
	fp.Hit(ctx, fp.Nope)
	_ = fp.Armed(ctx, name)
	_ = fp.Armed(ctx, fp.AfterSign)
}
`)
	write("q/q.go", "package q\n\nfunc f() { faultpoint.Hit(nil, \"not the import\") }\n")
	write("p/testdata/bad.go", "package bad\n\nimport \""+importPath+"\"\n\nfunc f() { faultpoint.Hit(nil, \"x\") }\n")
	write(".hidden/bad.go", "package bad\n\nimport \""+importPath+"\"\n\nfunc f() { faultpoint.Hit(nil, \"x\") }\n")
	write("notes.txt", "faultpoint.Hit(ctx, \"x\")\n")

	s, err := scanCallSites(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"p/p.go:11", "p/p.go:12", "p/p.go:13"}; !slices.Equal(s.bad, want) {
		t.Fatalf("bad = %v, want %v", s.bad, want)
	}
	if want := []string{"p/p.go:10", "p/p.go:14"}; !slices.Equal(s.sites, want) {
		t.Fatalf("sites = %v, want %v", s.sites, want)
	}

	write("broken.go", "package broken\n\nfunc {\n")
	if _, err := scanCallSites(root); err == nil || !strings.Contains(err.Error(), "parse broken.go") {
		t.Fatalf("scan of an unparsable file = %v, want a parse error", err)
	}
}
