package flows

import (
	"go/ast"
	"path"
	"strconv"
	"strings"
)

const modulesDir = "internal/modules/"

type directCall struct {
	name string
	via  string
	file string
	line int
}

func CheckStackScriptsUseTheBus(flows []Flow, env Env) []Problem {
	pkg, problems := loadScriptPackage(env)
	for _, f := range flows {
		if f.Status.AtLeastBuilt() {
			problems = append(problems, pkg.directCallProblems(f)...)
		}
	}
	return problems
}

func (pkg *scriptPackage) directCallProblems(f Flow) []Problem {
	var problems []Problem
	for _, command := range f.Commands {
		if kind, _ := f.TriggerKind(command); kind != TriggerConsumer {
			continue
		}
		for _, o := range f.Outcomes {
			script := ScriptName(f, command, o)
			if pkg.builders[script] == nil {
				continue
			}
			if call, ok := pkg.firstDirectCall(script); ok {
				problems = append(problems, Problem{
					File: call.file,
					Line: call.line,
					Msg: "flow " + f.ID + " script " + script + " calls " + call.name + " in process" + call.via +
						"; consumer flows run against the real worker, so drive the trigger through the bus " +
						"(the real event, or a scenario step that causes it) instead of calling a module's " +
						"app or adapters package",
				})
			}
		}
	}
	return problems
}

func (pkg *scriptPackage) firstDirectCall(script string) (directCall, bool) {
	seen := map[string]bool{}
	var walk func(fn, via string) (directCall, bool)
	walk = func(fn, via string) (directCall, bool) {
		if seen[fn] {
			return directCall{}, false
		}
		seen[fn] = true
		modules := moduleImports(pkg.fileOf[fn])
		var found directCall
		var hit bool
		ast.Inspect(pkg.builders[fn].decl.Body, func(n ast.Node) bool {
			if hit {
				return false
			}
			if name, ok := moduleReference(n, modules); ok {
				pos := pkg.fset.Position(n.Pos())
				found, hit = directCall{name: name, via: via, file: pos.Filename, line: pos.Line}, true
				return false
			}
			ident, ok := n.(*ast.Ident)
			if !ok || pkg.builders[ident.Name] == nil {
				return true
			}
			next := via
			if next == "" {
				next = " (through " + ident.Name + ")"
			}
			found, hit = walk(ident.Name, next)
			return !hit
		})
		return found, hit
	}
	return walk(script, "")
}

func moduleReference(n ast.Node, modules map[string]string) (string, bool) {
	var expr ast.Expr
	literal := false
	switch node := n.(type) {
	case *ast.CallExpr:
		expr = node.Fun
	case *ast.CompositeLit:
		expr, literal = node.Type, true
	default:
		return "", false
	}
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	kind, ok := modules[pkgIdent.Name]
	if !ok || (literal && kind != "adapters") {
		return "", false
	}
	return pkgIdent.Name + "." + sel.Sel.Name, true
}

func moduleImports(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, spec := range file.Imports {
		value, _ := strconv.Unquote(spec.Path.Value)
		if !isModuleCallSurface(value) {
			continue
		}
		name := path.Base(value)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		out[name] = path.Base(value)
	}
	return out
}

func isModuleCallSurface(importPath string) bool {
	_, rest, ok := strings.Cut(importPath, modulesDir)
	if !ok {
		return false
	}
	parts := strings.Split(rest, "/")
	return len(parts) == 2 && (parts[1] == "app" || parts[1] == "adapters")
}
