package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var lowerPattern = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)

const (
	mainWithoutNATS = "testkit.Main(m)"
	mainWithNATS    = "testkit.Main(m, testkit.WithNATS())"
)

func planConsumer(root *os.Root, modPath string, args []string) (Plan, error) {
	module, name := args[0], args[1]
	if err := requireModule(root, module, name, lowerPattern); err != nil {
		return Plan{}, err
	}
	d := newData(modPath, module, name)
	d.Handler = string(unicode.ToUpper(rune(name[0]))) + name[1:]
	dir := moduleDir(module)
	entry := strings.TrimSpace(render("consumer/entry.tmpl", d))
	adapters := `"` + modPath + "/internal/modules/" + module + `/adapters"`
	return Plan{
		Create: renderAll(d, map[string]string{
			filepath.Join(dir, "adapters", d.File+".go"): "consumer/consumer.go.tmpl",
			filepath.Join(dir, d.File+"_test.go"):        "consumer/consumer_test.go.tmpl",
		}),
		Edit: map[string]func(string) (string, error){
			filepath.Join(dir, "module.go"): func(old string) (string, error) {
				return appendToReturn(withDeps(withImport(old, adapters)), "Consumers", entry)
			},
			filepath.Join(dir, "main_test.go"): func(old string) (string, error) {
				return strings.Replace(old, mainWithoutNATS, mainWithNATS, 1), nil
			},
		},
	}, nil
}

func withImport(src, path string) string {
	if strings.Contains(src, path) {
		return src
	}
	return strings.Replace(src, "import (\n", "import (\n\t"+path+"\n", 1)
}

func withDeps(src string) string {
	return strings.NewReplacer(
		"type Module struct{}",
		"type Module struct {\n\tdeps module.Deps\n}",
		"func New(module.Deps) *Module { return &Module{} }",
		"func New(d module.Deps) *Module { return &Module{deps: d} }",
		"func (*Module) Consumers() []bus.Consumer",
		"func (m *Module) Consumers() []bus.Consumer",
	).Replace(src)
}

func appendToReturn(src, method, entry string) (string, error) {
	const op = "gen.appendToReturn"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if err != nil {
		return "", invalid(op, "parse: %v", err)
	}
	var lit *ast.CompositeLit
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != method || fn.Body == nil || len(fn.Body.List) != 1 {
			continue
		}
		if ret, ok := fn.Body.List[0].(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
			lit, _ = ret.Results[0].(*ast.CompositeLit)
		}
	}
	if lit == nil {
		return "", invalid(op, "%s must be a single return of a slice literal", method)
	}
	if len(lit.Elts) == 0 {
		at := fset.Position(lit.Rbrace).Offset
		return gofmt(src[:at] + "\n" + entry + ",\n" + src[at:]), nil
	}
	at := fset.Position(lit.Elts[len(lit.Elts)-1].End()).Offset
	return gofmt(src[:at] + ",\n" + entry + src[at:]), nil
}
