package scripts_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var modeArgIndex = map[string]int{"WriteFile": 2, "OpenFile": 2, "Chmod": 1}

func directExecutableWrites(fset *token.FileSet, file *ast.File) []token.Position {
	var found []token.Position
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			return n.Name.Name != "writeExecutable"
		case *ast.CallExpr:
			if hasExecutableMode(n) {
				found = append(found, fset.Position(n.Pos()))
			}
		}
		return true
	})
	return found
}

func hasExecutableMode(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	i, known := modeArgIndex[sel.Sel.Name]
	if !ok || pkg.Name != "os" || !known || i >= len(call.Args) {
		return false
	}
	lit, ok := call.Args[i].(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return false
	}
	mode, err := strconv.ParseUint(lit.Value, 0, 32)
	return err == nil && mode&0o111 != 0
}

func TestScriptsTests_writeExecutablesThroughForkLockedHelper(t *testing.T) {
	t.Run("detector", func(t *testing.T) {
		for _, tc := range []struct {
			name, src string
			flagged   bool
		}{
			{"os.WriteFile with 0o755", "func f() { os.WriteFile(p, b, 0o755) }", true},
			{"os.WriteFile with a legacy 0755", "func f() { os.WriteFile(p, b, 0755) }", true},
			{"os.OpenFile with 0o700", "func f() { os.OpenFile(p, os.O_CREATE, 0o700) }", true},
			{"os.Chmod with 0o500", "func f() { os.Chmod(p, 0o500) }", true},
			{"os.WriteFile with 0o644", "func f() { os.WriteFile(p, b, 0o644) }", false},
			{"WriteFile on another receiver", "func f() { r.WriteFile(p, b, 0o755) }", false},
			{"os.WriteFile inside writeExecutable", "func writeExecutable() { os.WriteFile(p, b, 0o700) }", false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				fset := token.NewFileSet()
				file, err := parser.ParseFile(fset, "planted_test.go", "package p\n\n"+tc.src+"\n", parser.SkipObjectResolution)
				if err != nil {
					t.Fatal(err)
				}
				if found := directExecutableWrites(fset, file); (len(found) > 0) != tc.flagged {
					t.Fatalf("flagged %v, want %v: %v", found, tc.flagged, tc.src)
				}
			})
		}
	})

	t.Run("scripts tests", func(t *testing.T) {
		err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
				return err
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			for _, pos := range directExecutableWrites(fset, file) {
				t.Errorf("%s: write the executable with writeExecutable, which holds syscall.ForkLock so no fork can copy the open descriptor (golang/go#22315)", pos)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}
