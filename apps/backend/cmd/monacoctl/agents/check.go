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
	"slices"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	checkBudget  = 60 * time.Second
	excerptLines = 8
)

var (
	testFuncRE = regexp.MustCompile(`(?m)^func (Test\w+)\(t \*testing\.T\)`)
	shebangRE  = regexp.MustCompile(`^#!(/usr/bin/env\s+|\S*/)(ba|z)?sh\b`)
)

type checkRow struct {
	label  string
	dir    string
	cmds   [][]string
	goJSON bool
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
	rows, err := env.stage0(ctx, base)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "stage 0 on tree %s (base %s)\n", tree[:12], base)
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

func (env *Env) stage0(ctx context.Context, base string) ([]checkRow, error) {
	out, err := env.Run(ctx, env.Work, "", "git", "diff", "--name-only", "--diff-filter=d", base+"...HEAD")
	if err != nil {
		return nil, fmt.Errorf("diff against %s: %w", base, err)
	}
	changed := strings.Fields(string(out))
	var rows []checkRow
	if slices.ContainsFunc(changed, func(f string) bool { return strings.HasPrefix(f, "apps/backend/") }) {
		goRows, err := env.goRows(ctx, base)
		if err != nil {
			return nil, err
		}
		rows = append(rows, goRows...)
	}
	rows = append(rows, env.shellRows(changed)...)
	rows = append(rows, env.testFileRows(changed)...)
	if slices.ContainsFunc(changed, func(f string) bool { return strings.HasPrefix(f, "packages/mobile-core/") }) {
		rows = append(rows, checkRow{
			label: "swift test", dir: filepath.Join(env.Work, "packages", "mobile-core"),
			cmds: [][]string{{"swift", "test"}},
		})
	}
	return rows, nil
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
	return []checkRow{
		{label: "go build", dir: backend, cmds: [][]string{append([]string{"go", "build"}, pkgs...)}},
		{label: "go vet", dir: backend, cmds: [][]string{append([]string{"go", "vet"}, pkgs...)}},
		{
			label: "go test -short", dir: backend, goJSON: true,
			cmds: [][]string{append([]string{"go", "test", "-short", "-count=1", "-json"}, pkgs...)},
		},
	}, nil
}

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
	syntax := checkRow{label: "bash -n", dir: env.Work}
	for _, s := range scripts {
		syntax.cmds = append(syntax.cmds, []string{"bash", "-n", s})
	}
	lint := checkRow{label: "shellcheck", dir: env.Work, cmds: [][]string{append([]string{"shellcheck"}, scripts...)}}
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
		row := checkRow{label: "scripts tests", dir: filepath.Join(env.Work, "scripts")}
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
			label: "python tests", dir: env.Work,
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
	ctx, cancel := context.WithTimeout(ctx, checkBudget)
	defer cancel()
	for _, row := range rows {
		rowStart := r.env.Now()
		for _, cmd := range row.cmds {
			text, err := r.exec(ctx, row, cmd)
			if r.env.Now().Sub(r.start) > checkBudget {
				return r.overBudget(stdout, row)
			}
			if err != nil {
				_, _ = fmt.Fprintf(stdout, "  %-15s FAIL  %s\n", row.label, strings.Join(cmd, " "))
				for _, line := range excerpt(text + "\n" + err.Error()) {
					_, _ = fmt.Fprintf(stdout, "    %s\n", line)
				}
				return detailErr(errs.CodeInvalidInput, "monacoctl.agents.check", row.label+" failed; see the log")
			}
		}
		_, _ = fmt.Fprintf(stdout, "  %-15s ok    %.1fs\n", row.label, r.env.Now().Sub(rowStart).Seconds())
	}
	return nil
}

func (r *checkRun) exec(ctx context.Context, row checkRow, cmd []string) (string, error) {
	start := r.env.Now()
	_, _ = fmt.Fprintf(&r.log, "$ (cd %s && %s)\n", row.dir, strings.Join(cmd, " "))
	out, err := r.env.Run(ctx, row.dir, "", cmd[0], cmd[1:]...)
	text := string(out)
	if row.goJSON {
		var timings []timing
		text, timings = goTestTimings(out, r.env.Now())
		r.timings = append(r.timings, timings...)
	} else {
		r.timings = append(r.timings, timing{cmdName(row, cmd), r.env.Now().Sub(start)})
	}
	r.log.WriteString(text)
	if err != nil {
		_, _ = fmt.Fprintf(&r.log, "%v\n", err)
	}
	return text, err
}

func (r *checkRun) overBudget(stdout io.Writer, row checkRow) error {
	slowest := slices.MaxFunc(r.timings, func(a, b timing) int { return cmp.Compare(a.took, b.took) })
	_, _ = fmt.Fprintf(stdout, "  %-15s over budget\n", row.label)
	return detailErr(errs.CodeUpstreamTimeout, "monacoctl.agents.check", fmt.Sprintf(
		"over the %s budget after %.0fs; slowest: %s (%.1fs)",
		checkBudget, r.env.Now().Sub(r.start).Seconds(), slowest.name, slowest.took.Seconds()))
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
