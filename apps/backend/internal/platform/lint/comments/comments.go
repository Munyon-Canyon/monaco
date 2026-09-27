package comments

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	allowedPrefix = "//go:"
	recursive     = "/..."
)

type finding struct {
	path string
	line int
}

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		args = []string{"." + recursive}
	}
	var findings []finding
	for _, arg := range args {
		files, err := goFiles(arg)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl lint comments: %v\n", err)
			return 2
		}
		for _, path := range files {
			found, err := check(path)
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "monacoctl lint comments: %v\n", err)
				return 2
			}
			findings = append(findings, found...)
		}
	}
	for _, f := range findings {
		_, _ = fmt.Fprintf(stdout, "%s:%d: comment not allowed\n", f.path, f.line)
	}
	if len(findings) > 0 {
		return 1
	}
	return 0
}

func check(path string) ([]finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if ast.IsGenerated(file) {
		return nil, nil
	}
	var found []finding
	for _, group := range file.Comments {
		for _, c := range group.List {
			if !strings.HasPrefix(c.Text, allowedPrefix) {
				found = append(found, finding{path: path, line: fset.Position(c.Pos()).Line})
			}
		}
	}
	return found, nil
}

func goFiles(arg string) ([]string, error) {
	if root, ok := strings.CutSuffix(arg, recursive); ok {
		return walk(root)
	}
	info, err := os.Stat(arg)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	if !info.IsDir() {
		return []string{arg}, nil
	}
	entries, err := os.ReadDir(arg)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && isGoFile(e.Name()) {
			files = append(files, filepath.Join(arg, e.Name()))
		}
	}
	return files, nil
}

func walk(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && ignoredDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if isGoFile(d.Name()) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	return files, nil
}

func ignoredDir(name string) bool {
	return name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

func isGoFile(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "_")
}
