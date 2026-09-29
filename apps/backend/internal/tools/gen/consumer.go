package gen

import (
	"go/ast"
	"go/format"
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

func planConsumer(root *os.Root, args []string) (Plan, error) {
	module, name := args[0], args[1]
	if err := requireModule(root, module, name, lowerPattern); err != nil {
		return Plan{}, err
	}
	d, err := newData(root, module, name)
	if err != nil {
		return Plan{}, err
	}
	d.Handler = string(unicode.ToUpper(rune(name[0]))) + name[1:]
	dir := moduleDir(module)
	create, err := renderAll(d, map[string]string{
		filepath.Join(dir, "adapters", d.File+".go"): "consumer/consumer.go.tmpl",
		filepath.Join(dir, d.File+"_test.go"):        "consumer/consumer_test.go.tmpl",
	})
	if err != nil {
		return Plan{}, err
	}
	entry, err := render("consumer/entry.tmpl", d)
	if err != nil {
		return Plan{}, err
	}
	adapters := `"` + d.ModPath + "/internal/modules/" + module + `/adapters"`
	return Plan{Create: create, Edit: map[string]func(string) (string, error){
		filepath.Join(dir, "module.go"): func(old string) (string, error) {
			return appendToReturn(withImport(old, adapters), "Consumers", strings.TrimSpace(entry))
		},
		filepath.Join(dir, "main_test.go"): func(old string) (string, error) {
			return strings.Replace(old, mainWithoutNATS, mainWithNATS, 1), nil
		},
	}}, nil
}

func withImport(src, path string) string {
	if strings.Contains(src, path) {
		return src
	}
	return strings.Replace(src, "import (\n", "import (\n\t"+path+"\n", 1)
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
	at := fset.Position(lit.Rbrace).Offset
	out, err := format.Source([]byte(src[:at] + "\n" + entry + ",\n" + src[at:]))
	if err != nil {
		return "", invalid(op, "format: %v", err)
	}
	return string(out), nil
}
