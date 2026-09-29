package agents

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	excerptLines = 8
	packageKind  = "package"
	openAPISpec  = "apps/backend/api/openapi.yaml"
	vacuumLint   = "dshanley/vacuum:v0.30.6 lint -b -q -n warn -r /api/.vacuum.yaml /api/openapi.yaml"
)

var (
	testFuncRE = regexp.MustCompile(`(?m)^func (Test\w+)\(t \*testing\.T\)`)
	shebangRE  = regexp.MustCompile(`^#!(/usr/bin/env\s+|\S*/)(ba|z)?sh\b`)
)

type checkRow struct {
	label string
	kind  string
	dir   string
	cmds  [][]string
	skip  string
}

type timing struct {
	name string
	took time.Duration
}

type checkRun struct {
	env     *Env
	start   time.Time
	log     bytes.Buffer
	timings []timing
}

func checkCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	base := "origin/" + env.Config.FeatureBranch
	switch {
	case len(args) == 2 && args[0] == "--base":
		base = args[1]
	case len(args) != 0:
		return usageError("check [--base <ref>]")
	}
	tree, head, err := env.cleanHead(ctx)
	if err != nil {
		return err
	}
	if _, err := os.Stat(env.statePath("checks", tree)); err == nil {
		_, _ = fmt.Fprintf(stdout, "stage 0 already passed on tree %s\n", tree[:12])
		return nil
	}
	parent := env.stackParent(ctx, base)
	rows, err := env.stage0(ctx, base, parent, head)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "stage 0 on tree %s (base %s, parent %s)\n", tree[:12], base, parent)
	run := &checkRun{env: env, start: env.Now()}
	runErr := run.rows(ctx, rows, stdout)
	logPath, err := env.writeState("logs", "check-"+tree[:12]+".log", run.log.Bytes())
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "log: %s\n", logPath)
	if runErr != nil {
		return runErr
	}
	record, err := env.writeState("checks", tree, fmt.Appendf(nil, "head %s\nbase %s\n", head, base))
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "passed in %.1fs; recorded %s\n", env.Now().Sub(run.start).Seconds(), record)
	return nil
}

func (env *Env) cleanHead(ctx context.Context) (tree, head string, err error) {
	out, err := env.Run(ctx, env.Work, "", "git", "rev-parse", "HEAD^{tree}", "HEAD")
	if err != nil {
		return "", "", fmt.Errorf("read HEAD: %w", err)
	}
	tree, head, _ = strings.Cut(strings.TrimSpace(string(out)), "\n")
	dirty, err := env.Run(ctx, env.Work, "", "git", "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return "", "", fmt.Errorf("read the working tree: %w", err)
	}
	if len(bytes.TrimSpace(dirty)) > 0 {
		return "", "", detailErr(errs.CodeInvalidInput, "monacoctl.agents.check",
			"the working tree differs from HEAD; commit first, since check records HEAD's tree")
	}
	return tree, head, nil
}

func (env *Env) statePath(sub, name string) string {
	return filepath.Join(env.Common, "pstack", env.Config.Milestone, sub, name)
}

func (env *Env) writeState(sub, name string, body []byte) (string, error) {
	p := env.statePath(sub, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return "", fmt.Errorf("write %s: %w", p, err)
	}
	if err := os.WriteFile(p, body, 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", p, err)
	}
	return p, nil
}

func (env *Env) stackParent(ctx context.Context, base string) string {
	out, err := env.Run(ctx, env.Work, "", "gt", "parent", "--no-interactive")
	parent, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if err != nil || parent == "" || parent == env.Config.FeatureBranch {
		return base
	}
	return parent
}

func (env *Env) stage0(ctx context.Context, base, parent, head string) ([]checkRow, error) {
	out, err := env.Run(ctx, env.Work, "", "git", "diff", "--name-only", "--diff-filter=d", base+"...HEAD")
	if err != nil {
		return nil, fmt.Errorf("diff against %s: %w", base, err)
	}
	changed := strings.Fields(string(out))
	rows := env.prRows(parent, head)
	if slices.ContainsFunc(changed, func(f string) bool { return strings.HasPrefix(f, "apps/backend/") }) {
		goRows, err := env.goRows(ctx, base)
		if err != nil {
			return nil, err
		}
		rows = append(rows, goRows...)
	}
	rows = append(rows, env.shellRows(changed)...)
	rows = append(rows, env.testFileRows(changed)...)
	if slices.ContainsFunc(changed, func(f string) bool {
		return strings.HasPrefix(f, "packages/mobile-core/") || f == openAPISpec
	}) {
		rows = append(rows, checkRow{
			label: "swift test", kind: "swift", dir: filepath.Join(env.Work, "packages", "mobile-core"),
			cmds: [][]string{{"swift", "test", "-Xswiftc", "-warnings-as-errors"}},
		})
	}
	return env.pathRows(ctx, rows, changed, parent, head)
}

func (env *Env) prRows(parent, head string) []checkRow {
	vars := []string{"env", "BASE_SHA=" + parent, "HEAD_SHA=" + head, "PR_LABELS=[]", "python3"}
	return []checkRow{
		{label: "pr size", kind: "pr", dir: env.Work, cmds: [][]string{
			append(slices.Clone(vars), "scripts/check-pr-size.py"),
		}},
		{label: "gate changes", kind: "pr", dir: env.Work, cmds: [][]string{
			append(slices.Clone(vars), "scripts/check-gate-changes.py"),
		}},
	}
}

func (env *Env) pathRows(
	ctx context.Context,
	rows []checkRow,
	changed []string,
	parent, head string,
) ([]checkRow, error) {
	for _, r := range []struct {
		paths []string
		build func() (checkRow, error)
	}{
		{
			[]string{"apps/backend/", "scripts/ci/ready.sh", "scripts/gen-docs.sh", "scripts/install-sqlc.sh"},
			env.readyRow,
		},
		{
			[]string{
				"apps/backend/migrations/", "apps/backend/atlas.hcl", "apps/backend/.atlas-version",
				"scripts/install-atlas.sh",
			},
			env.migrateRow,
		},
		{
			[]string{
				openAPISpec, "apps/backend/api/.vacuum.yaml", "scripts/ci/oasdiff-breaking.sh",
				"scripts/ci/oasdiff-breaking-test.sh", "scripts/ci/oasdiff-levels.txt",
			},
			func() (checkRow, error) { return env.openAPIRow(ctx, parent, head) },
		},
		{[]string{"docs/", "mkdocs.yml", "requirements-docs.txt", openAPISpec}, env.docsRow},
	} {
		if !slices.ContainsFunc(changed, func(f string) bool { return underAny(f, r.paths) }) {
			continue
		}
		row, err := r.build()
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func underAny(file string, paths []string) bool {
	return slices.ContainsFunc(paths, func(p string) bool {
		return file == p || strings.HasSuffix(p, "/") && strings.HasPrefix(file, p)
	})
}

func (env *Env) readyRow() (checkRow, error) {
	return checkRow{
		label: "ready", kind: "ready", dir: env.Work,
		cmds: append(env.installUnlessPresent("sqlc"), []string{"scripts/ci/ready.sh"}),
	}, nil
}

func (env *Env) migrateRow() (checkRow, error) {
	return checkRow{
		label: "migrate lint", kind: "migrate", dir: filepath.Join(env.Work, "apps", "backend"),
		cmds: append(env.installUnlessPresent("atlas"), []string{"go", "run", "./cmd/monacoctl", "migrate", "lint"}),
	}, nil
}

func (env *Env) installUnlessPresent(tool string) [][]string {
	if isFile(filepath.Join(env.Work, ".bin", tool)) {
		return nil
	}
	return [][]string{{filepath.Join(env.Work, "scripts", "install-"+tool+".sh")}}
}

func isFile(name string) bool {
	info, err := os.Stat(name)
	return err == nil && info.Mode().IsRegular()
}

func (env *Env) openAPIRow(ctx context.Context, parent, head string) (checkRow, error) {
	api := filepath.Join(env.Work, "apps", "backend", "api")
	row := checkRow{label: "openapi", kind: "openapi", dir: env.Work, cmds: [][]string{
		slices.Concat([]string{"docker", "run", "--rm", "-v", api + ":/api:ro"}, strings.Fields(vacuumLint)),
		{"scripts/ci/oasdiff-breaking-test.sh"},
	}}
	spec := env.specAt(ctx, parent)
	if spec == nil {
		return row, nil
	}
	file, err := env.writeState("openapi", head[:12]+".yaml", spec)
	if err != nil {
		return checkRow{}, err
	}
	row.cmds = append(row.cmds, []string{"scripts/ci/oasdiff-breaking.sh", file, openAPISpec})
	return row, nil
}

func (env *Env) specAt(ctx context.Context, ref string) []byte {
	spec, err := env.Run(ctx, env.Work, "", "git", "show", ref+":"+openAPISpec)
	if err != nil {
		return nil
	}
	return spec
}

func (env *Env) docsRow() (checkRow, error) {
	row := checkRow{label: "mkdocs", kind: "docs", dir: env.Work}
	for _, root := range []string{env.Work, filepath.Dir(env.Common)} {
		if bin := filepath.Join(root, ".venv", "bin", "mkdocs"); isFile(bin) {
			row.cmds = [][]string{{"env", "NO_MKDOCS_2_WARNING=true", bin, "build", "--strict", "--site-dir", "site"}}
			return row, nil
		}
	}
	row.skip = "no .venv/bin/mkdocs here or in the main checkout; README's docs site row installs it"
	return row, nil
}

func (env *Env) goRows(ctx context.Context, base string) ([]checkRow, error) {
	backend := filepath.Join(env.Work, "apps", "backend")
	self, _ := os.Executable()
	out, err := env.Run(ctx, backend, "", self, "ci", "affected", "--base", base)
	if err != nil {
		return nil, fmt.Errorf("find affected packages: %w", err)
	}
	pkgs := strings.Fields(string(out))
	if len(pkgs) == 0 {
		return nil, nil
	}
	records, err := env.records()
	if err != nil {
		return nil, err
	}
	running := len(slices.DeleteFunc(records, func(r Record) bool { return r.State == Exited }))
	p := strconv.Itoa(testParallelism(runtime.NumCPU(), running))
	tags := []string{"-tags", "faultpoints"}
	lint, err := env.lintRow(ctx, backend, pkgs)
	if err != nil {
		return nil, err
	}
	return []checkRow{
		{
			label: "go build", kind: "go", dir: backend,
			cmds: [][]string{slices.Concat([]string{"go", "build"}, tags, buildable(backend, pkgs))},
		},
		{label: "go vet", kind: "go", dir: backend, cmds: [][]string{slices.Concat([]string{"go", "vet"}, tags, pkgs)}},
		lint,
		{
			label: "go test -short", kind: packageKind, dir: backend,
			cmds: [][]string{slices.Concat([]string{"go", "test"}, tags, []string{
				"-short", "-count=1", "-timeout", env.Config.Budget[packageKind].String(), "-p", p, "-json",
			}, pkgs)},
		},
	}, nil
}

func (env *Env) lintRow(ctx context.Context, backend string, pkgs []string) (checkRow, error) {
	pin, err := os.ReadFile(filepath.Join(backend, ".golangci-lint-version"))
	if err != nil {
		return checkRow{}, fmt.Errorf("read the golangci-lint pin: %w", err)
	}
	want := strings.TrimSpace(string(pin))
	out, err := env.Run(ctx, backend, "", "golangci-lint", "version", "--short")
	have := "v" + strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
	if err != nil {
		have = "none"
	}
	if have != want {
		install := "go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@" + want
		return checkRow{}, detailErr(errs.CodeInvalidInput, "monacoctl.agents.check",
			fmt.Sprintf("golangci-lint on PATH is %s and CI pins %s; run: %s", have, want, install))
	}
	return checkRow{label: "go lint", kind: "lint", dir: backend, cmds: [][]string{
		slices.Concat([]string{"golangci-lint", "run"}, pkgs),
		slices.Concat([]string{"go", "run", "./internal/platform/lint/nogo/cmd/nogo"}, pkgs),
		{"go", "run", "./cmd/monacoctl", "lint", "comments"},
	}}, nil
}

func testParallelism(cpus, running int) int {
	return max(2, cpus/max(1, running))
}

func buildable(backend string, pkgs []string) []string {
	return slices.DeleteFunc(slices.Clone(pkgs), func(pkg string) bool {
		files, _ := filepath.Glob(filepath.Join(backend, pkg, "*.go"))
		return len(files) > 0 && !slices.ContainsFunc(files, isSource)
	})
}

func isSource(file string) bool { return !strings.HasSuffix(file, "_test.go") }

func (env *Env) shellRows(changed []string) []checkRow {
	var scripts []string
	for _, f := range changed {
		if strings.HasSuffix(f, ".sh") || env.hasShellShebang(f) {
			scripts = append(scripts, f)
		}
	}
	if len(scripts) == 0 {
		return nil
	}
	syntax := checkRow{label: "bash -n", kind: "shell", dir: env.Work}
	for _, s := range scripts {
		syntax.cmds = append(syntax.cmds, []string{"bash", "-n", s})
	}
	lint := checkRow{
		label: "shellcheck",
		kind:  "shell",
		dir:   env.Work,
		cmds:  [][]string{append([]string{"shellcheck"}, scripts...)},
	}
	return []checkRow{syntax, lint}
}

func (env *Env) hasShellShebang(file string) bool {
	if path.Ext(file) != "" {
		return false
	}
	f, err := os.Open(filepath.Join(env.Work, file))
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	line, _ := bufio.NewReader(f).ReadString('\n')
	return shebangRE.MatchString(line)
}

func (env *Env) testFileRows(changed []string) []checkRow {
	goTests, pyTests := env.affectedTests(changed)
	var rows []checkRow
	if len(goTests) > 0 {
		row := checkRow{label: "scripts tests", kind: "scripts", dir: filepath.Join(env.Work, "scripts")}
		for _, pkg := range slices.Sorted(maps.Keys(goTests)) {
			run := "^(" + strings.Join(goTests[pkg], "|") + ")$"
			row.cmds = append(
				row.cmds,
				[]string{"go", "test", "-short", "-count=1", "-run", run, "./" + strings.TrimPrefix(pkg, ".")},
			)
		}
		rows = append(rows, row)
	}
	if len(pyTests) > 0 {
		rows = append(rows, checkRow{
			label: "python tests", kind: "python", dir: env.Work,
			cmds: [][]string{append([]string{"python3", "-m", "unittest"}, pyTests...)},
		})
	}
	return rows
}

func (env *Env) affectedTests(changed []string) (goTests map[string][]string, pyTests []string) {
	goTests = map[string][]string{}
	scripts := os.DirFS(filepath.Join(env.Work, "scripts"))
	_ = fs.WalkDir(scripts, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.Name() == "testdata" {
			return fs.SkipDir
		}
		isGo := strings.HasSuffix(p, "_test.go")
		isPy := strings.HasPrefix(d.Name(), "test_") && strings.HasSuffix(p, ".py")
		if !isGo && !isPy {
			return nil
		}
		body, _ := fs.ReadFile(scripts, p)
		if !slices.ContainsFunc(changed, func(f string) bool {
			return f == "scripts/"+p || bytes.Contains(body, []byte(`"`+path.Base(f)+`"`))
		}) {
			return nil
		}
		if isPy {
			pyTests = append(pyTests, "scripts/"+p)
			return nil
		}
		for _, m := range testFuncRE.FindAllSubmatch(body, -1) {
			goTests[path.Dir(p)] = append(goTests[path.Dir(p)], string(m[1]))
		}
		return nil
	})
	return goTests, pyTests
}

func (r *checkRun) rows(ctx context.Context, rows []checkRow, stdout io.Writer) error {
	for _, row := range rows {
		if err := r.row(ctx, row, stdout); err != nil {
			return err
		}
	}
	return nil
}

func (r *checkRun) row(ctx context.Context, row checkRow, stdout io.Writer) error {
	if row.skip != "" {
		_, _ = fmt.Fprintf(stdout, "  %-15s skip  %s\n", row.label, row.skip)
		_, _ = fmt.Fprintf(&r.log, "skip %s: %s\n", row.label, row.skip)
		return nil
	}
	budget := r.env.Config.Budget[row.kind]
	if row.kind != packageKind {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, budget)
		defer cancel()
	}
	rowStart, first := r.env.Now(), len(r.timings)
	warnings := 0
	for _, cmd := range row.cmds {
		text, err := r.exec(ctx, row, cmd)
		warnings += strings.Count(text, "::warning ")
		if err := r.overBudget(stdout, row, budget, rowStart, r.timings[first:]); err != nil {
			return err
		}
		if err != nil {
			_, _ = fmt.Fprintf(stdout, "  %-15s FAIL  %s\n", row.label, strings.Join(cmd, " "))
			for _, line := range excerpt(text + "\n" + err.Error()) {
				_, _ = fmt.Fprintf(stdout, "    %s\n", line)
			}
			return detailErr(errs.CodeInvalidInput, "monacoctl.agents.check", row.label+" failed; see the log")
		}
	}
	note := ""
	if warnings > 0 {
		note = fmt.Sprintf("  %d warnings in the log", warnings)
	}
	_, _ = fmt.Fprintf(stdout, "  %-15s ok    %.1fs%s\n", row.label, r.env.Now().Sub(rowStart).Seconds(), note)
	return nil
}

func (r *checkRun) exec(ctx context.Context, row checkRow, cmd []string) (string, error) {
	start := r.env.Now()
	_, _ = fmt.Fprintf(&r.log, "$ (cd %s && %s)\n", row.dir, strings.Join(cmd, " "))
	out, err := r.env.Run(ctx, row.dir, "", cmd[0], cmd[1:]...)
	text := string(out)
	timings := []timing{{cmdName(row, cmd), r.env.Now().Sub(start)}}
	if row.kind == packageKind {
		text, timings = goTestTimings(out, r.env.Now())
	}
	r.timings = append(r.timings, timings...)
	r.log.WriteString(text)
	if err != nil {
		_, _ = fmt.Fprintf(&r.log, "%v\n", err)
	}
	return text, err
}

func (r *checkRun) overBudget(
	stdout io.Writer, row checkRow, budget time.Duration, rowStart time.Time, timings []timing,
) error {
	var detail string
	if row.kind == packageKind {
		slow := slices.DeleteFunc(slices.Clone(timings), func(t timing) bool { return t.took < budget })
		if len(slow) == 0 {
			return nil
		}
		s := slowest(slow)
		detail = fmt.Sprintf("%s: package %s took %.1fs, over the %s per-package budget",
			row.label, s.name, s.took.Seconds(), budget)
	} else {
		took := r.env.Now().Sub(rowStart)
		if took <= budget {
			return nil
		}
		s := slowest(timings)
		detail = fmt.Sprintf("%s row over the %s %s budget after %.0fs; slowest: %s (%.1fs)",
			row.label, budget, row.kind, took.Seconds(), s.name, s.took.Seconds())
	}
	_, _ = fmt.Fprintf(stdout, "  %-15s over budget\n", row.label)
	return detailErr(errs.CodeUpstreamTimeout, "monacoctl.agents.check", detail)
}

func slowest(timings []timing) timing {
	return slices.MaxFunc(timings, func(a, b timing) int { return cmp.Compare(a.took, b.took) })
}

func cmdName(row checkRow, cmd []string) string {
	if len(row.cmds) == 1 {
		return row.label
	}
	return row.label + " " + cmd[len(cmd)-1]
}

type testEvent struct {
	Time    time.Time `json:"Time"`
	Action  string    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test"`
	Output  string    `json:"Output"`
	Elapsed float64   `json:"Elapsed"`
}

func goTestTimings(out []byte, now time.Time) (string, []timing) {
	var text strings.Builder
	started := map[string]time.Time{}
	var order []string
	var timings []timing
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		var e testEvent
		if json.Unmarshal([]byte(line), &e) != nil {
			text.WriteString(line + "\n")
			continue
		}
		text.WriteString(e.Output)
		if e.Test != "" {
			continue
		}
		switch e.Action {
		case "start":
			started[e.Package] = e.Time
			order = append(order, e.Package)
		case "pass", "fail", "skip":
			delete(started, e.Package)
			timings = append(timings, timing{pkgName(e.Package), time.Duration(e.Elapsed * float64(time.Second))})
		}
	}
	for _, p := range order {
		if at, running := started[p]; running {
			timings = append(timings, timing{pkgName(p), now.Sub(at)})
		}
	}
	return text.String(), timings
}

func pkgName(importPath string) string {
	if _, rel, ok := strings.Cut(importPath, "/apps/backend/"); ok {
		return "./" + rel
	}
	return importPath
}

func excerpt(text string) []string {
	var lines, fails []string
	for line := range strings.SplitSeq(strings.TrimSpace(text), "\n") {
		line = strings.TrimRight(line, " \t")
		lines = append(lines, line)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--- FAIL") || strings.HasPrefix(trimmed, "FAIL") ||
			strings.HasPrefix(trimmed, "panic:") || strings.Contains(trimmed, ".go:") {
			fails = append(fails, line)
		}
	}
	if len(fails) > 0 {
		lines = fails
	}
	if len(lines) > excerptLines {
		lines = lines[len(lines)-excerptLines:]
	}
	return lines
}
