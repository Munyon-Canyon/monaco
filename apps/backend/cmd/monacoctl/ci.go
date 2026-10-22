package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const (
	ciUsage      = "usage: monacoctl ci affected --base <ref> [--excluded]"
	allPackages  = "./..."
	scanSentinel = "scansModuleSource"
)

type scanRule struct {
	roots   []string
	added   *regexp.Regexp
	scanner string
}

func scanRules() []scanRule {
	return []scanRule{
		{
			roots:   []string{"internal/modules", "internal/platform"},
			added:   regexp.MustCompile(`^\+.*\bobservability\.(Info|Warn|Error|Debug)\(`),
			scanner: "internal/platform/observability",
		},
	}
}

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
		excluded := fs.Bool("excluded", false, "")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *base == "" {
			_, _ = fmt.Fprintln(stderr, ciUsage)
			return 2
		}
		pkgs, skipped, err := env.affected(context.Background(), *base)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl ci affected: %v\n", err)
			return 1
		}
		if *excluded {
			pkgs = skipped
		}
		for _, p := range pkgs {
			_, _ = fmt.Fprintln(stdout, p)
		}
		return 0
	}
}

func (env ciEnv) affected(ctx context.Context, base string) (selected, excluded []string, err error) {
	const op = "monacoctl.ciAffected"
	diff, err := env.exec(ctx, env.moduleDir, nil, env.gitBin,
		"diff", "--name-only", "--relative", base+"...HEAD", "--", ".")
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	rows, err := env.exec(ctx, env.moduleDir, nil, env.gitBin,
		"diff", "--name-only", base+"...HEAD", "--", ":(top)"+flows.Dir)
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	changed := slices.Concat(strings.Fields(string(diff)), strings.Fields(string(rows)))
	if len(changed) == 0 {
		return nil, nil, nil
	}
	listed, err := env.exec(ctx, env.moduleDir, nil, env.goBin,
		"list", "-deps", "-json=ImportPath,Imports,TestImports,XTestImports,Module", allPackages)
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	pkgs, err := decodePackages(listed)
	if err != nil {
		return nil, nil, err
	}
	scanners, err := env.scanners(ctx, base, changed)
	if err != nil {
		return nil, nil, err
	}
	selected, excluded = reverseDeps(pkgs, changed, scanners...)
	return selected, excluded, nil
}

func (env ciEnv) scanners(ctx context.Context, base string, changed []string) ([]string, error) {
	const op = "monacoctl.ciScanners"
	var dirs []string
	for _, rule := range scanRules() {
		specs := make([]string, len(rule.roots))
		for i, root := range rule.roots {
			specs[i] = ":(glob)" + root + "/**/*.go"
		}
		diff, err := env.exec(ctx, env.moduleDir, nil, env.gitBin,
			slices.Concat([]string{"diff", "-U0", "--relative", base + "...HEAD", "--"}, specs)...)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, op)
		}
		if slices.ContainsFunc(strings.Split(string(diff), "\n"), rule.added.MatchString) {
			dirs = append(dirs, rule.scanner)
		}
	}
	if !slices.ContainsFunc(changed, func(f string) bool { return strings.HasSuffix(f, ".go") }) {
		return dirs, nil
	}
	files, err := env.exec(ctx, env.moduleDir, nil, env.gitBin, "grep", "-l", scanSentinel, "--", "*_test.go")
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	for _, f := range strings.Fields(string(files)) {
		dirs = append(dirs, path.Dir(f))
	}
	return dirs, nil
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

func flowsReaders() []string {
	return []string{
		"cmd/gen",
		"cmd/monacoctl",
		"cmd/monacoctl/agents",
		"cmd/monacoctl/verify",
		"internal/testkit/flows",
		"internal/tools/flows",
		"internal/tools/gen",
	}
}

func agentTooling() []string {
	return []string{"cmd/monacoctl", "cmd/monacoctl/agents", "cmd/monacoctl/verify"}
}

func sharedTooling(dir string) bool {
	return slices.ContainsFunc([]string{"internal/platform", "internal/events"}, func(root string) bool {
		return dir == root || strings.HasPrefix(dir, root+"/")
	})
}

func reverseDeps(byDir map[string]listedPackage, changed []string, scanners ...string) (selected, excluded []string) {
	hit, local, bare, ok := seedPackages(byDir, changed)
	if !ok {
		return []string{allPackages}, nil
	}
	seedScanners(byDir, bare, scanners)
	seeds := maps.Clone(hit)
	growToImporters(byDir, hit)
	dirOf := map[string]string{}
	for dir, p := range byDir {
		dirOf[p.ImportPath] = dir
	}
	for dir, p := range byDir {
		tests := slices.Concat(p.TestImports, p.XTestImports)
		testsHit := slices.ContainsFunc(tests, func(i string) bool { return hit[i] })
		switch {
		case !hit[p.ImportPath] && !local[p.ImportPath] && !bare[p.ImportPath] && !testsHit:
		case slices.Contains(agentTooling(), dir) && !inputsChanged(p, dirOf, seeds, local):
			excluded = append(excluded, "./"+dir)
		default:
			selected = append(selected, "./"+dir)
		}
	}
	slices.Sort(selected)
	slices.Sort(excluded)
	return selected, excluded
}

func seedScanners(byDir map[string]listedPackage, bare map[string]bool, scanners []string) {
	for _, dir := range scanners {
		if p, found := byDir[dir]; found {
			bare[p.ImportPath] = true
		}
	}
}

func growToImporters(byDir map[string]listedPackage, hit map[string]bool) {
	for grew := true; grew; {
		grew = false
		for _, p := range byDir {
			if !hit[p.ImportPath] && slices.ContainsFunc(p.Imports, func(i string) bool { return hit[i] }) {
				hit[p.ImportPath], grew = true, true
			}
		}
	}
}

func inputsChanged(p listedPackage, dirOf map[string]string, seeds, local map[string]bool) bool {
	return seeds[p.ImportPath] || local[p.ImportPath] ||
		slices.ContainsFunc(p.Imports, func(i string) bool { return seeds[i] && !sharedTooling(dirOf[i]) })
}

func seedPackages(byDir map[string]listedPackage, changed []string) (hit, local, bare map[string]bool, ok bool) {
	hit, local, bare = map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, f := range changed {
		if shared, modules, scoped := moduleScoped(byDir, f); scoped {
			for _, dir := range append(modules, shared) {
				bare[byDir[dir].ImportPath] = true
			}
			continue
		}
		dirs := seedDirs(byDir, f)
		if len(dirs) == 0 {
			return nil, nil, nil, false
		}
		into := hit
		if testOnly(f) {
			into = local
		}
		for _, d := range dirs {
			p, found := byDir[d]
			if !found {
				return nil, nil, nil, false
			}
			into[p.ImportPath] = true
		}
	}
	return hit, local, bare, true
}

func moduleScoped(byDir map[string]listedPackage, f string) (shared string, modules []string, scoped bool) {
	shared, area := moduleArea(f)
	if _, found := byDir[shared]; !found || area == "" {
		return "", nil, false
	}
	if dirs := moduleDirs(byDir, area); len(dirs) > 0 {
		return shared, dirs, true
	}
	if shared == "internal/errs" {
		return shared, moduleDirs(byDir, ""), true
	}
	return "", nil, false
}

func moduleArea(f string) (shared, area string) {
	dir, name := path.Split(f)
	shared = strings.TrimSuffix(path.Clean(dir), "/testdata")
	if strings.HasSuffix(name, "_test.go") {
		return shared, ""
	}
	stem := strings.TrimSuffix(strings.TrimSuffix(name, ".go"), ".gen")
	switch {
	case shared == "internal/platform/observability" && strings.HasSuffix(name, ".go"):
		area = strings.TrimPrefix(stem, "msgs_")
	case shared == "internal/events" && strings.HasSuffix(name, ".go"):
		area = strings.TrimSuffix(stem, "_registrations")
	case shared == "internal/errs" && strings.HasSuffix(name, ".go"):
		area = strings.TrimPrefix(stem, "codes_")
	case shared == "internal/errs" && strings.HasSuffix(name, ".golden"):
		stem = strings.TrimSuffix(name, ".golden")
		area = strings.TrimPrefix(stem, "codes_")
	}
	if area == stem {
		return shared, ""
	}
	return shared, area
}

func cutLast(s, sep string) (before, after string, found bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

func moduleDirs(byDir map[string]listedPackage, area string) []string {
	var dirs []string
	for dir := range byDir {
		module, found := strings.CutPrefix(dir, "internal/modules/")
		if !found {
			continue
		}
		module, _, _ = strings.Cut(module, "/")
		if area == "" || module == area {
			dirs = append(dirs, dir)
		}
	}
	if trimmed, _, found := cutLast(area, "_"); len(dirs) == 0 && found {
		return moduleDirs(byDir, trimmed)
	}
	return dirs
}

func testOnly(f string) bool {
	return strings.HasSuffix(f, "_test.go") || strings.HasPrefix(f, "internal/events/testdata/")
}

func seedDirs(byDir map[string]listedPackage, f string) []string {
	query, isQuery := strings.CutPrefix(f, "queries/")
	module, _, nested := strings.Cut(query, "/")
	switch {
	case f == "sqlc.yaml":
		return sqlcDirs(byDir)
	case strings.HasPrefix(f, flows.Dir+"/"):
		return flowsReaders()
	case isQuery && nested:
		return []string{"internal/modules/" + module + "/sqlc"}
	case runsEverything(f):
		return nil
	case strings.HasSuffix(f, ".go"):
		return []string{path.Dir(f)}
	}
	dir := path.Dir(f)
	for byDir[dir].ImportPath == "" && dir != "." {
		dir = path.Dir(dir)
	}
	return []string{dir}
}

func runsEverything(f string) bool {
	return f == "go.mod" || f == "go.sum" || f == ".golangci.yml"
}

func sqlcDirs(byDir map[string]listedPackage) []string {
	var dirs []string
	for dir := range byDir {
		if path.Base(dir) == "sqlc" {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}
