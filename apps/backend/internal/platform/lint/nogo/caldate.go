package nogo

import (
	_ "embed"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

//go:embed caldate.allow
var caldateAllow string

func CaldateAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "caldate",
		Doc:  "reports a time.Date with a literal year of 2000 or later in tests when it is compared with time.Now or passed to a db or repo method: stamp fixtures relative to the clock",
		Run:  runCaldate,
	}
}

func runCaldate(pass *analysis.Pass) (any, error) {
	allowed, err := allowFrom(caldateAllow)
	if err != nil {
		return nil, err
	}
	for _, file := range pass.Files {
		filename := pass.Fset.File(file.Pos()).Name()
		if !strings.HasSuffix(filename, "_test.go") || allowedPath(allowed, filename) {
			continue
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				for _, date := range (&caldateFunc{pass: pass}).staleDates(fn.Body) {
					pass.Reportf(
						date.Pos(),
						"calendar date %s compared with the clock; stamp it relative to time.Now() or a testkit clock",
						types.ExprString(date),
					)
				}
			}
		}
	}
	return nil, nil
}

type caldateFunc struct {
	pass  *analysis.Pass
	dates map[types.Object]map[*ast.CallExpr]bool
	nows  map[types.Object]bool
}

func (c *caldateFunc) staleDates(body *ast.BlockStmt) []*ast.CallExpr {
	c.dates = map[types.Object]map[*ast.CallExpr]bool{}
	c.nows = map[types.Object]bool{}
	c.trackAssignments(body)

	stale := map[*ast.CallExpr]bool{}
	clocked := map[*ast.CallExpr]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.BinaryExpr:
			if isComparison(n.Op) {
				c.flowsTogether(stale, n.X, n.Y)
			}
		case *ast.CallExpr:
			c.callSinks(n, stale, clocked)
		}
		return true
	})
	var out []*ast.CallExpr
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && stale[call] && !clocked[call] {
			out = append(out, call)
		}
		return true
	})
	return out
}

func (c *caldateFunc) trackAssignments(body *ast.BlockStmt) {
	pairs := assignments(body)
	for changed := true; changed; {
		changed = false
		for _, pair := range pairs {
			changed = c.bind(pair[0], pair[1]) || changed
		}
	}
}

func assignments(body *ast.BlockStmt) [][2]ast.Expr {
	var pairs [][2]ast.Expr
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			pairs = append(pairs, zip(n.Lhs, n.Rhs)...)
		case *ast.ValueSpec:
			names := make([]ast.Expr, len(n.Names))
			for i, name := range n.Names {
				names[i] = name
			}
			pairs = append(pairs, zip(names, n.Values)...)
		}
		return true
	})
	return pairs
}

func zip(lhs, rhs []ast.Expr) [][2]ast.Expr {
	if len(lhs) != len(rhs) {
		return nil
	}
	pairs := make([][2]ast.Expr, len(lhs))
	for i := range lhs {
		pairs[i] = [2]ast.Expr{lhs[i], rhs[i]}
	}
	return pairs
}

func (c *caldateFunc) bind(lhs, rhs ast.Expr) bool {
	id, ok := lhs.(*ast.Ident)
	if !ok {
		return false
	}
	obj := c.pass.TypesInfo.ObjectOf(id)
	if obj == nil {
		return false
	}
	changed := false
	for date := range c.dateOrigins(rhs) {
		if c.dates[obj] == nil {
			c.dates[obj] = map[*ast.CallExpr]bool{}
		}
		if !c.dates[obj][date] {
			c.dates[obj][date] = true
			changed = true
		}
	}
	if !c.nows[obj] && c.hasNow(rhs) {
		c.nows[obj] = true
		changed = true
	}
	return changed
}

func (c *caldateFunc) dateOrigins(e ast.Expr) map[*ast.CallExpr]bool {
	out := map[*ast.CallExpr]bool{}
	ast.Inspect(e, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			if c.isModernDate(n) {
				out[n] = true
			}
		case *ast.Ident:
			for date := range c.dates[c.pass.TypesInfo.ObjectOf(n)] {
				out[date] = true
			}
		}
		return true
	})
	return out
}

func (c *caldateFunc) hasNow(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			if fn := typeutil.StaticCallee(c.pass.TypesInfo, n); fn != nil && fn.Pkg() != nil &&
				fn.Pkg().Path() == "time" && fn.Name() == "Now" && fn.Type().(*types.Signature).Recv() == nil {
				found = true
			}
		case *ast.Ident:
			found = found || c.nows[c.pass.TypesInfo.ObjectOf(n)]
		}
		return !found
	})
	return found
}

func (c *caldateFunc) isModernDate(call *ast.CallExpr) bool {
	fn := typeutil.StaticCallee(c.pass.TypesInfo, call)
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != "time" || fn.Name() != "Date" || len(call.Args) == 0 {
		return false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return false
	}
	year, err := strconv.Atoi(lit.Value)
	return err == nil && year >= 2000
}

func (c *caldateFunc) flowsTogether(stale map[*ast.CallExpr]bool, a, b ast.Expr) {
	for _, pair := range [2][2]ast.Expr{{a, b}, {b, a}} {
		if c.hasNow(pair[1]) {
			for date := range c.dateOrigins(pair[0]) {
				stale[date] = true
			}
		}
	}
}

func (c *caldateFunc) callSinks(call *ast.CallExpr, stale, clocked map[*ast.CallExpr]bool) {
	fn := typeutil.StaticCallee(c.pass.TypesInfo, call)
	if fn == nil || fn.Pkg() == nil {
		return
	}
	sig := fn.Type().(*types.Signature)
	path := fn.Pkg().Path()
	recv := receiverOf(call, sig)
	switch {
	case strings.Contains(path, "/testkit"):
		c.mark(clocked, append(slices.Clone(call.Args), recv...)...)
	case path == "time" && recv != nil:
		c.compareWithReceiver(stale, fn.Name(), recv[0], call.Args)
	case path == "time" && (fn.Name() == "Since" || fn.Name() == "Until"):
		c.mark(stale, call.Args...)
	case recv != nil && storesTimes(path):
		c.mark(stale, timeArgs(call, sig)...)
	}
}

func (c *caldateFunc) compareWithReceiver(stale map[*ast.CallExpr]bool, method string, recv ast.Expr, args []ast.Expr) {
	if !slices.Contains([]string{"Before", "After", "Sub", "Equal", "Compare"}, method) {
		return
	}
	for _, arg := range args {
		c.flowsTogether(stale, recv, arg)
	}
}

func (c *caldateFunc) mark(set map[*ast.CallExpr]bool, exprs ...ast.Expr) {
	for _, e := range exprs {
		for date := range c.dateOrigins(e) {
			set[date] = true
		}
	}
}

func receiverOf(call *ast.CallExpr, sig *types.Signature) []ast.Expr {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sig.Recv() != nil {
		return []ast.Expr{sel.X}
	}
	return nil
}

func timeArgs(call *ast.CallExpr, sig *types.Signature) []ast.Expr {
	var out []ast.Expr
	for i, arg := range call.Args {
		if i < sig.Params().Len() && isTimeTime(sig.Params().At(i).Type()) {
			out = append(out, arg)
		}
	}
	return out
}

func storesTimes(pkgPath string) bool {
	rel, ok := strings.CutPrefix(pkgPath, modulePath+"/internal/")
	if !ok {
		return false
	}
	if rel == "platform/db" || strings.HasPrefix(rel, "platform/db/") {
		return true
	}
	return strings.HasPrefix(rel, "modules/") && (rel[strings.LastIndex(rel, "/")+1:] == "repo")
}

func isTimeTime(t types.Type) bool {
	named, ok := t.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "time" && named.Obj().Name() == "Time"
}

func isComparison(op token.Token) bool {
	return slices.Contains([]token.Token{token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ}, op)
}
