package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	ciUsage     = "usage: monacoctl ci affected --base <ref>"
	allPackages = "./..."
)

type ciEnv struct {
	moduleDir, goBin, gitBin string
	exec                     execFunc
}

type listedPackage struct {
	ImportPath   string   `json:"ImportPath"`
	Imports      []string `json:"Imports"`
	TestImports  []string `json:"TestImports"`
	XTestImports []string `json:"XTestImports"`
	Module       *struct {
		Path string `json:"Path"`
		Main bool   `json:"Main"`
	} `json:"Module"`
}

func toolCi(_ toolEnv) tool {
	return ciTool(ciEnv{moduleDir: ".", goBin: "go", gitBin: "git", exec: runCommand})
}

func ciTool(env ciEnv) tool {
	return func(args []string, stdout, stderr io.Writer) int {
		if len(args) == 0 || args[0] != "affected" {
			_, _ = fmt.Fprintln(stderr, ciUsage)
			return 2
		}
		fs := flag.NewFlagSet("ci affected", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		base := fs.String("base", "", "")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *base == "" {
			_, _ = fmt.Fprintln(stderr, ciUsage)
			return 2
		}
		pkgs, err := env.affected(context.Background(), *base)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl ci affected: %v\n", err)
			return 1
		}
		for _, p := range pkgs {
			_, _ = fmt.Fprintln(stdout, p)
		}
		return 0
	}
}

func (env ciEnv) affected(ctx context.Context, base string) ([]string, error) {
	const op = "monacoctl.ciAffected"
	diff, err := env.exec(ctx, env.moduleDir, nil, env.gitBin,
		"diff", "--name-only", "--relative", base+"...HEAD", "--", ".")
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	changed := strings.Fields(string(diff))
	if len(changed) == 0 {
		return nil, nil
	}
	listed, err := env.exec(ctx, env.moduleDir, nil, env.goBin,
		"list", "-deps", "-json=ImportPath,Imports,TestImports,XTestImports,Module", allPackages)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	pkgs, err := decodePackages(listed)
	if err != nil {
		return nil, err
	}
	return reverseDeps(pkgs, changed), nil
}

func decodePackages(data []byte) (map[string]listedPackage, error) {
	byDir := map[string]listedPackage{}
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var p listedPackage
		err := dec.Decode(&p)
		if errors.Is(err, io.EOF) {
			return byDir, nil
		}
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, "monacoctl.decodePackages")
		}
		if p.Module == nil || !p.Module.Main {
			continue
		}
		byDir[path.Clean("."+strings.TrimPrefix(p.ImportPath, p.Module.Path))] = p
	}
}

func reverseDeps(byDir map[string]listedPackage, changed []string) []string {
	hit := map[string]bool{}
	for _, f := range changed {
		p, ok := byDir[path.Dir(f)]
		if !ok || !strings.HasSuffix(f, ".go") || strings.HasPrefix(f, "internal/testkit/") {
			return []string{allPackages}
		}
		hit[p.ImportPath] = true
	}
	for grew := true; grew; {
		grew = false
		for _, p := range byDir {
			if !hit[p.ImportPath] && slices.ContainsFunc(p.Imports, func(i string) bool { return hit[i] }) {
				hit[p.ImportPath], grew = true, true
			}
		}
	}
	var out []string
	for dir, p := range byDir {
		tests := slices.Concat(p.TestImports, p.XTestImports)
		if hit[p.ImportPath] || slices.ContainsFunc(tests, func(i string) bool { return hit[i] }) {
			out = append(out, "./"+dir)
		}
	}
	slices.Sort(out)
	return out
}
