package agents

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
)

func widenToFuncs(dir, ranges string) string {
	var out strings.Builder
	for line := range strings.Lines(ranges) {
		file, span, _ := strings.Cut(strings.TrimSpace(line), ":")
		from, to, _ := strings.Cut(span, "-")
		start, _ := strconv.Atoi(from)
		end, _ := strconv.Atoi(to)
		start, end = enclosingFuncs(filepath.Join(dir, file), start, end)
		fmt.Fprintf(&out, "%s:%d-%d\n", file, start, end)
	}
	return out.String()
}

func enclosingFuncs(path string, start, end int) (int, int) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return start, end
	}
	from, to := start, end
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		first, last := fset.Position(fn.Pos()).Line, fset.Position(fn.End()).Line
		if start <= last && end >= first {
			from, to = min(from, first), max(to, last)
		}
	}
	return from, to
}
