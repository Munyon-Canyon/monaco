package gen

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"unicode"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

//go:embed templates
var templates embed.FS

var modulePattern = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

type problem string

func (p problem) Error() string { return string(p) }

func invalid(op, format string, args ...any) error {
	return errs.Wrap(problem(fmt.Sprintf(format, args...)), errs.CodeInvalidInput, op)
}

type Generator struct {
	Kind       string
	Args       []string
	plan       func(root *os.Root, args []string) (Plan, error)
	regenerate bool
}

func (g Generator) Usage() string { return "gen " + g.Kind + " " + strings.Join(g.Args, " ") }

func Generators() []Generator {
	return []Generator{
		{Kind: "module", Args: []string{"<name>"}, plan: planModule, regenerate: true},
	}
}

func Find(kind string) (Generator, bool) {
	all := Generators()
	i := slices.IndexFunc(all, func(g Generator) bool { return g.Kind == kind })
	if i < 0 {
		return Generator{}, false
	}
	return all[i], true
}

type Plan struct {
	Create map[string]string
	Edit   map[string]func(old string) (string, error)
}

func (g Generator) Run(ctx context.Context, dir string, args []string) ([]string, error) {
	touched, err := g.write(dir, args)
	if err != nil || !g.regenerate {
		return touched, err
	}
	return touched, Regenerate(ctx, dir)
}

func (g Generator) write(dir string, args []string) ([]string, error) {
	const op = "gen.write"
	if len(args) != len(g.Args) {
		return nil, invalid(op, "usage: %s", g.Usage())
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	defer func() { _ = root.Close() }()
	p, err := g.plan(root, args)
	if err != nil {
		return nil, err
	}
	edited, err := p.edits(root)
	if err != nil {
		return nil, err
	}
	var touched []string
	for _, files := range []map[string]string{p.Create, edited} {
		for _, rel := range slices.Sorted(maps.Keys(files)) {
			if err := writeFile(root, rel, files[rel]); err != nil {
				return touched, err
			}
			touched = append(touched, rel)
		}
	}
	return touched, nil
}

func (p Plan) edits(root *os.Root) (map[string]string, error) {
	const op = "gen.edits"
	for rel := range p.Create {
		if _, err := root.Stat(rel); !errors.Is(err, fs.ErrNotExist) {
			return nil, invalid(op, "%s already exists", rel)
		}
	}
	edited := make(map[string]string, len(p.Edit))
	for rel, edit := range p.Edit {
		old, err := root.ReadFile(rel)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInvalidInput, op, slog.String("file", rel))
		}
		if edited[rel], err = edit(string(old)); err != nil {
			return nil, errs.Wrap(err, errs.CodeInvalidInput, op, slog.String("file", rel))
		}
	}
	return edited, nil
}

func writeFile(root *os.Root, rel, body string) error {
	const op = "gen.writeFile"
	if err := root.MkdirAll(filepath.Dir(rel), 0o750); err != nil {
		return errs.Wrap(err, errs.CodeInternal, op, slog.String("file", rel))
	}
	if err := root.WriteFile(rel, []byte(body), 0o600); err != nil {
		return errs.Wrap(err, errs.CodeInternal, op, slog.String("file", rel))
	}
	return nil
}

func Regenerate(ctx context.Context, dir string) error {
	const op = "gen.Regenerate"
	for _, cmd := range []*exec.Cmd{
		exec.CommandContext(ctx, "go", "run", "-trimpath", "./scripts/gen-golangci", "."),
		exec.CommandContext(ctx, "go", "run", "-trimpath", "./scripts/gen-registry", "."),
	} {
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return errs.Wrap(
				problem(string(out)),
				errs.CodeInternal,
				op,
				slog.String("cmd", cmd.String()),
				slog.Any("err", err),
			)
		}
	}
	return nil
}

type data struct {
	ModPath string
	Module  string
	Name    string
	File    string
}

func newData(root *os.Root, module, name string) (data, error) {
	modPath, err := modulePath(root)
	if err != nil {
		return data{}, err
	}
	return data{ModPath: modPath, Module: module, Name: name, File: snake(name)}, nil
}

func modulePath(root *os.Root) (string, error) {
	const op = "gen.modulePath"
	mod, err := root.ReadFile("go.mod")
	if err != nil {
		return "", errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	for line := range strings.Lines(string(mod)) {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(path), nil
		}
	}
	return "", invalid(op, "go.mod has no module line")
}

func render(name string, d data) (string, error) {
	const op = "gen.render"
	tmpl, err := template.ParseFS(templates, "templates/"+name)
	if err != nil {
		return "", errs.Wrap(err, errs.CodeInternal, op, slog.String("template", name))
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, d); err != nil {
		return "", errs.Wrap(err, errs.CodeInternal, op, slog.String("template", name))
	}
	if !strings.HasSuffix(name, ".go.tmpl") {
		return out.String(), nil
	}
	src, err := format.Source(out.Bytes())
	if err != nil {
		return "", errs.Wrap(err, errs.CodeInternal, op, slog.String("template", name))
	}
	return string(src), nil
}

func renderAll(d data, files map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(files))
	for rel, tmpl := range files {
		body, err := render(tmpl, d)
		if err != nil {
			return nil, err
		}
		out[rel] = body
	}
	return out, nil
}

func snake(name string) string {
	var b strings.Builder
	for i, r := range name {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

func moduleDir(module string) string { return filepath.Join("internal", "modules", module) }
