package observability

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	observabilityPath = "github.com/monaco/monaco/apps/backend/internal/platform/observability"
	boundaryPath      = observabilityPath + "/boundary"
	moduleRoot        = "../../.."
)

func parseRegistry(t *testing.T) (map[string]Msg, []string) {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "msgs.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	vars := map[string]Msg{}
	var listed []string
	for _, decl := range f.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
			for _, spec := range gen.Specs {
				listed = append(listed, readVarSpec(t, spec.(*ast.ValueSpec), vars)...)
			}
		}
	}
	return vars, listed
}

func readVarSpec(t *testing.T, vs *ast.ValueSpec, vars map[string]Msg) []string {
	t.Helper()
	var listed []string
	for i, name := range vs.Names {
		lit := vs.Values[i].(*ast.CompositeLit)
		if name.Name != "registry" {
			vars[name.Name] = msgLiteral(t, lit)
			continue
		}
		for _, elt := range lit.Elts {
			listed = append(listed, elt.(*ast.Ident).Name)
		}
	}
	return listed
}

func msgLiteral(t *testing.T, lit *ast.CompositeLit) Msg {
	t.Helper()
	var m Msg
	for _, elt := range lit.Elts {
		kv := elt.(*ast.KeyValueExpr)
		switch kv.Key.(*ast.Ident).Name {
		case "Name":
			m.Name = unquote(t, kv.Value)
		case "Required":
			for _, k := range kv.Value.(*ast.CompositeLit).Elts {
				m.Required = append(m.Required, unquote(t, k))
			}
		}
	}
	return m
}

func unquote(t *testing.T, e ast.Expr) string {
	t.Helper()
	s, err := strconv.Unquote(e.(*ast.BasicLit).Value)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type callSiteChecker struct {
	fset *token.FileSet
	vars map[string]Msg
}

type fileScope struct {
	observability string
	boundary      string
	inObs         bool
	inBoundary    bool
}

func scopeOf(f *ast.File) fileScope {
	s := fileScope{inObs: f.Name.Name == "observability", inBoundary: f.Name.Name == "boundary"}
	for _, imp := range f.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		name := filepath.Base(path)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		switch path {
		case observabilityPath:
			s.observability = name
		case boundaryPath:
			s.boundary = name
		}
	}
	return s
}

func (s fileScope) isHelper(fun ast.Expr) bool {
	switch fn := fun.(type) {
	case *ast.SelectorExpr:
		x, ok := fn.X.(*ast.Ident)
		return ok && (x.Name == s.observability && isInfoOrDebug(fn.Sel.Name) ||
			x.Name == s.boundary && isWarnOrError(fn.Sel.Name))
	case *ast.Ident:
		return s.inObs && isInfoOrDebug(fn.Name) || s.inBoundary && isWarnOrError(fn.Name)
	}
	return false
}

func isInfoOrDebug(name string) bool { return name == "Info" || name == "Debug" }

func isWarnOrError(name string) bool { return name == "Warn" || name == "Error" }

func (s fileScope) msgName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.SelectorExpr:
		if pkg, ok := x.X.(*ast.Ident); ok && pkg.Name == s.observability {
			return x.Sel.Name
		}
	case *ast.Ident:
		if s.inObs {
			return x.Name
		}
	}
	return ""
}

func literalKeys(args []ast.Expr) map[string]bool {
	keys := map[string]bool{}
	for _, arg := range args {
		call, ok := arg.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			continue
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		lit, isLit := call.Args[0].(*ast.BasicLit)
		if ok && isLit && lit.Kind == token.STRING && isIdent(sel.X, "slog") {
			k, _ := strconv.Unquote(lit.Value)
			keys[k] = true
		}
	}
	return keys
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

func (c callSiteChecker) check(f *ast.File) []string {
	scope := scopeOf(f)
	var findings []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 || !scope.isHelper(call.Fun) {
			return true
		}
		pos := c.fset.Position(call.Pos())
		at := strings.TrimPrefix(pos.Filename, moduleRoot+"/") + ":" + strconv.Itoa(pos.Line)
		m, ok := c.vars[scope.msgName(call.Args[1])]
		if !ok {
			findings = append(findings, at+": message is not a registered observability.Msg")
			return true
		}
		have := literalKeys(call.Args[2:])
		for _, k := range m.Required {
			if !have[k] {
				findings = append(findings, fmt.Sprintf("%s: %s is missing required attr %q", at, m.Name, k))
			}
		}
		return true
	})
	return findings
}

func TestRegistry_listsEveryMessageOnceWithUniqueNames(t *testing.T) {
	t.Parallel()
	vars, listed := parseRegistry(t)
	declared := make([]string, 0, len(vars))
	for name := range vars {
		declared = append(declared, name)
	}
	slices.Sort(declared)
	if sorted := slices.Sorted(slices.Values(listed)); !slices.Equal(sorted, declared) {
		t.Fatalf("registry lists %v, msgs.go declares %v; every Msg var must be listed exactly once", sorted, declared)
	}
	valid := regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
	seen := map[string]bool{}
	for _, m := range registry {
		if seen[m.Name] || !valid.MatchString(m.Name) {
			t.Errorf("message name %q is duplicated or not a dotted lower_snake identifier", m.Name)
		}
		seen[m.Name] = true
	}
}

func TestRegistry_everyCallSiteInTheModuleIsRegistered(t *testing.T) {
	t.Parallel()
	vars, _ := parseRegistry(t)
	c := callSiteChecker{fset: token.NewFileSet(), vars: vars}
	var findings []string
	err := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".") && path != moduleRoot) {
			return filepath.SkipDir
		}
		if d.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		f, err := parser.ParseFile(c.fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		findings = append(findings, c.check(f)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) > 0 {
		t.Fatalf("log calls that break the message registry (register the Msg in msgs.go and pass its attrs):\n%s",
			strings.Join(findings, "\n"))
	}
}

func TestRegistryCheck_flagsPlantedViolations(t *testing.T) {
	t.Parallel()
	vars, _ := parseRegistry(t)
	c := callSiteChecker{fset: token.NewFileSet(), vars: vars}
	f, err := parser.ParseFile(c.fset, "testdata/registry/planted.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"testdata/registry/planted.go:13: message is not a registered observability.Msg",
		"testdata/registry/planted.go:14: message is not a registered observability.Msg",
		"testdata/registry/planted.go:15: message is not a registered observability.Msg",
		`testdata/registry/planted.go:16: boot.stopped is missing required attr "err"`,
		`testdata/registry/planted.go:19: boot.listening is missing required attr "service"`,
		`testdata/registry/planted.go:19: boot.listening is missing required attr "addr"`,
	}
	if got := c.check(f); !slices.Equal(got, want) {
		t.Fatalf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
