package nogo

import (
	_ "embed"
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

//go:embed wallclock.allow
var wallclockAllow string

func WallclockAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "wallclock",
		Doc:  "reports time.Sleep, time.After, time.NewTicker, and time.Now in tests unless the test runs inside testing/synctest or uses the testkit clock",
		Run:  runWallclock,
	}
}

func runWallclock(pass *analysis.Pass) (any, error) {
	allowed, err := allowFrom(wallclockAllow)
	if err != nil {
		return nil, err
	}
	for _, file := range pass.Files {
		filename := pass.Fset.File(file.Pos()).Name()
		if !strings.HasSuffix(filename, "_test.go") {
			continue
		}
		parents := parentsOf(file)
		bubbles := synctestBubbles(pass, file)
		clocked := clockedFuncs(pass, file)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := wallClockName(pass, call)
			if name == "" || insideBubble(parents, bubbles, call) || clocked[enclosingFunc(parents, call)] {
				return true
			}
			if allowedPath(allowed, filename) {
				return true
			}
			pass.Reportf(call.Pos(), "wall clock time.%s: use testing/synctest or the testkit clock", name)
			return true
		})
	}
	return nil, nil
}

type allowLineError struct {
	line int
	msg  string
}

func (e allowLineError) Error() string {
	return fmt.Sprintf("wallclock: allow line %d: %s", e.line, e.msg)
}

func allowFrom(text string) (map[string]string, error) {
	allowed := map[string]string{}
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		path, reason, ok := strings.Cut(line, " ")
		reason = strings.TrimSpace(reason)
		if !ok || reason == "" {
			return nil, allowLineError{line: i + 1, msg: "want <path> <reason>"}
		}
		if _, dup := allowed[path]; dup {
			return nil, allowLineError{line: i + 1, msg: "duplicate " + path}
		}
		allowed[path] = reason
	}
	return allowed, nil
}

func allowedPath(allowed map[string]string, filename string) bool {
	slash := filepath.ToSlash(filename)
	for path := range allowed {
		if slash == path || strings.HasSuffix(slash, "/"+path) {
			return true
		}
	}
	return false
}

func wallClockName(pass *analysis.Pass, call *ast.CallExpr) string {
	fn, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "time" {
		return ""
	}
	if fn.Type().(*types.Signature).Recv() != nil {
		return ""
	}
	switch fn.Name() {
	case "Sleep", "After", "NewTicker", "Now":
		return fn.Name()
	default:
		return ""
	}
}

func parentsOf(file *ast.File) map[ast.Node]ast.Node {
	parents := map[ast.Node]ast.Node{}
	var stack []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) > 0 {
			parents[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		return true
	})
	return parents
}

func synctestBubbles(pass *analysis.Pass, file *ast.File) map[*ast.FuncLit]bool {
	bubbles := map[*ast.FuncLit]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn, ok := typeutil.Callee(pass.TypesInfo, call).(*types.Func)
		if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "testing/synctest" || fn.Name() != "Test" {
			return true
		}
		for _, arg := range call.Args {
			if lit, ok := arg.(*ast.FuncLit); ok {
				bubbles[lit] = true
			}
		}
		return true
	})
	return bubbles
}

func insideBubble(parents map[ast.Node]ast.Node, bubbles map[*ast.FuncLit]bool, n ast.Node) bool {
	for p := parents[n]; p != nil; p = parents[p] {
		if lit, ok := p.(*ast.FuncLit); ok && bubbles[lit] {
			return true
		}
	}
	return false
}

func enclosingFunc(parents map[ast.Node]ast.Node, n ast.Node) *ast.FuncDecl {
	for p := parents[n]; p != nil; p = parents[p] {
		if fn, ok := p.(*ast.FuncDecl); ok {
			return fn
		}
	}
	return nil
}

func clockedFuncs(pass *analysis.Pass, file *ast.File) map[*ast.FuncDecl]bool {
	out := map[*ast.FuncDecl]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !usesTestkitClock(pass, fn) {
			continue
		}
		out[fn] = true
	}
	return out
}

func usesTestkitClock(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	if obj := pass.TypesInfo.Defs[fn.Name]; obj != nil && typeUsesTestkitClock(obj.Type()) {
		return true
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		expr, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		if typeUsesTestkitClock(pass.TypesInfo.TypeOf(expr)) {
			found = true
			return false
		}
		return true
	})
	return found
}

func typeUsesTestkitClock(t types.Type) bool {
	if t == nil {
		return false
	}
	switch u := t.(type) {
	case *types.Pointer:
		return typeUsesTestkitClock(u.Elem())
	case *types.Named:
		obj := u.Obj()
		return obj.Pkg() != nil && obj.Pkg().Path() == modulePath+"/internal/testkit" && obj.Name() == "Clock"
	case *types.Signature:
		return tupleUsesTestkitClock(u.Params()) || tupleUsesTestkitClock(u.Results())
	default:
		return false
	}
}

func tupleUsesTestkitClock(tuple *types.Tuple) bool {
	for i := range tuple.Len() {
		if typeUsesTestkitClock(tuple.At(i).Type()) {
			return true
		}
	}
	return false
}
