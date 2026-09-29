package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	mutationUsage   = "usage: monacoctl mutation [--base main | --all] [--list | --pkg dir [--report file]]"
	mutantsAllow    = "mutants.allow"
	testOutputLines = 40
	lived           = "LIVED"
	killed          = "KILLED"
	timedOut        = "TIMED OUT"
)

type mutationEnv struct {
	moduleDir, goBin, gitBin, gremlins, tmpDir string
	exec                                       execFunc
	now                                        func() time.Time
}

type execFunc func(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error)

func runCommand(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		err = fmt.Errorf("%w: %s", err, bytes.TrimSpace(exit.Stderr))
	}
	return out, err
}

type mutationArgs struct {
	base, pkg, report string
	all, list         bool
}

type gremlinsReport struct {
	Files []struct {
		Name      string `json:"file_name"`
		Mutations []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
		} `json:"mutations"`
	} `json:"files"`
}

func mutationTool(env mutationEnv) tool {
	return func(args []string, stdout, stderr io.Writer) int { return mutationCmd(env, args, stdout, stderr) }
}

func mutationCmd(env mutationEnv, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mutation", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var a mutationArgs
	fs.StringVar(&a.base, "base", "main", "")
	fs.BoolVar(&a.all, "all", false, "")
	fs.BoolVar(&a.list, "list", false, "")
	fs.StringVar(&a.pkg, "pkg", "", "")
	fs.StringVar(&a.report, "report", "", "")
	if fs.Parse(args) != nil || fs.NArg() != 0 || a.list && a.pkg != "" || a.report != "" && a.pkg == "" {
		_, _ = fmt.Fprintln(stderr, mutationUsage)
		return 2
	}
	survivors, err := env.run(context.Background(), a, stdout)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl mutation: %v\n", err)
		return 1
	}
	for _, s := range survivors {
		_, _ = fmt.Fprintf(
			stderr,
			"monacoctl mutation: %s survived; kill it with a test or list it in %s with a reason\n",
			s,
			mutantsAllow,
		)
	}
	if len(survivors) > 0 {
		return 1
	}
	return 0
}

func (env mutationEnv) run(ctx context.Context, a mutationArgs, stdout io.Writer) ([]string, error) {
	const op = "monacoctl.mutation"
	exclude, err := os.ReadFile(filepath.Join(env.moduleDir, coverageExclude))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	listed, err := env.goList(ctx)
	if err != nil {
		return nil, err
	}
	var changed []string
	diffRef := ""
	if !a.all {
		if changed, err = env.changedFiles(ctx, a.base); err != nil {
			return nil, err
		}
		diffRef = a.base
	}
	dirs := affectedPackages(listed, changed, a.all, strings.Fields(string(exclude)))
	if a.list {
		data, _ := json.Marshal(dirs)
		_, _ = fmt.Fprintf(stdout, "%s\n", data)
		return nil, nil
	}
	if a.pkg != "" {
		if !slices.Contains(dirs, a.pkg) {
			return nil, errs.Wrap(unchangedPackageError(a.pkg), errs.CodeInvalidInput, op)
		}
		dirs = []string{a.pkg}
	}
	allowed, err := readAllowFile(filepath.Join(env.moduleDir, mutantsAllow))
	if err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintf(stdout, "mutating %d packages: %s\n", len(dirs), strings.Join(dirs, " "))
	var total mutantCounts
	start := env.now()
	defer func() { total.print(stdout, env.now().Sub(start)) }()
	var survivors []string
	for _, dir := range dirs {
		report, err := env.unleash(ctx, dir, diffRef, a.report)
		if err != nil {
			return nil, err
		}
		if pkg := total.add(report); pkg.timedOut > pkg.tested() {
			return nil, errs.Wrap(timedOutError(dir), errs.CodeInternal, op)
		}
		survivors = append(survivors, survivingMutants(dir, report, allowed)...)
	}
	return survivors, nil
}

func (env mutationEnv) goList(ctx context.Context) ([]string, error) {
	out, err := env.exec(ctx, env.moduleDir, nil, env.goBin,
		"list", "-f", `{{.Module.Dir}}	{{.Dir}}	{{len .GoFiles}}`, "./...")
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "monacoctl.goList")
	}
	var dirs []string
	for line := range strings.Lines(string(out)) {
		f := strings.SplitN(strings.TrimRight(line, "\n"), "\t", 3)
		if f[2] == "0" {
			continue
		}
		dirs = append(dirs, filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(f[1], f[0]), "/")))
	}
	return dirs, nil
}

func (env mutationEnv) changedFiles(ctx context.Context, base string) ([]string, error) {
	out, err := env.exec(
		ctx,
		env.moduleDir,
		nil,
		env.gitBin,
		"diff",
		"--name-only",
		"--relative",
		base+"...HEAD",
		"--",
		".",
	)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "monacoctl.changedFiles")
	}
	return strings.Fields(string(out)), nil
}

func affectedPackages(pkgs, changed []string, all bool, exclude []string) []string {
	changedDirs := map[string]bool{}
	for _, f := range changed {
		if strings.HasSuffix(f, ".go") {
			changedDirs[path.Dir(f)] = true
		}
	}
	dirs := []string{}
	for _, p := range pkgs {
		if (all || changedDirs[p]) && !excluded(p+"/", exclude) {
			dirs = append(dirs, p)
		}
	}
	slices.Sort(dirs)
	return dirs
}

func (env mutationEnv) unleash(ctx context.Context, dir, diffRef, out string) (gremlinsReport, error) {
	const op = "monacoctl.unleash"
	if out == "" {
		file, err := os.CreateTemp(env.tmpDir, "gremlins-*.json")
		if err != nil {
			return gremlinsReport{}, errs.Wrap(err, errs.CodeInternal, op)
		}
		out = file.Name()
		_ = file.Close()
		defer func() { _ = os.Remove(out) }()
	}
	args := []string{
		"unleash",
		"--silent",
		"--timeout-coefficient",
		"50",
		"--output",
		out,
		"--exclude-files",
		`\.gen\.go$`,
	}
	if diffRef != "" {
		args = append(args, "--diff", diffRef)
	}
	relativeGitDiff := []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=diff.relative", "GIT_CONFIG_VALUE_0=true"}
	if _, err := env.exec(ctx, filepath.Join(env.moduleDir, dir), relativeGitDiff, env.gremlins, args...); err != nil {
		return gremlinsReport{}, errs.Wrap(fmt.Errorf("%w\n%s", err, env.testOutput(ctx, dir)), errs.CodeInternal, op)
	}
	data, _ := os.ReadFile(out)
	if len(bytes.TrimSpace(data)) == 0 {
		return gremlinsReport{}, nil
	}
	var report gremlinsReport
	if err := json.Unmarshal(data, &report); err != nil {
		return gremlinsReport{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return report, nil
}

type unchangedPackageError string

func (e unchangedPackageError) Error() string {
	return string(e) + " has no changed Go files to mutate; pick one from --list"
}

type timedOutError string

func (e timedOutError) Error() string {
	return "more mutants in " + string(e) + " timed out than were tested; rerun on a quieter machine"
}

func (env mutationEnv) testOutput(ctx context.Context, dir string) string {
	out, err := env.exec(ctx, env.moduleDir, nil, env.goBin, "test", "-count=1", "./"+dir)
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	lines = lines[max(0, len(lines)-testOutputLines):]
	if err != nil {
		lines = append(lines, err.Error())
	}
	return "go test ./" + dir + " for context:\n" + strings.Join(lines, "\n")
}

type mutantCounts struct{ packages, killed, lived, timedOut int }

func (c mutantCounts) tested() int { return c.killed + c.lived }

func (c *mutantCounts) add(report gremlinsReport) mutantCounts {
	pkg := mutantCounts{packages: 1}
	for _, f := range report.Files {
		for _, m := range f.Mutations {
			switch m.Status {
			case killed:
				pkg.killed++
			case lived:
				pkg.lived++
			case timedOut:
				pkg.timedOut++
			}
		}
	}
	c.packages += pkg.packages
	c.killed += pkg.killed
	c.lived += pkg.lived
	c.timedOut += pkg.timedOut
	return pkg
}

func (c mutantCounts) print(w io.Writer, wall time.Duration) {
	_, _ = fmt.Fprintf(w, "mutation summary: %d packages, %d tested, %d killed, %d lived, %d timed out, wall %s\n",
		c.packages, c.tested(), c.killed, c.lived, c.timedOut, wall.Round(time.Second))
}

func mutantKey(dir, file string, line, column int, mutator string) string {
	return fmt.Sprintf("%s:%d:%d %s", path.Join(dir, file), line, column, mutator)
}

func survivingMutants(dir string, report gremlinsReport, allowed map[string]bool) []string {
	var out []string
	for _, f := range report.Files {
		for _, m := range f.Mutations {
			key := mutantKey(dir, f.Name, m.Line, m.Column, m.Type)
			if m.Status == lived && !allowed[key] {
				out = append(out, key)
			}
		}
	}
	slices.Sort(out)
	return out
}

func readAllowFile(name string) (map[string]bool, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "monacoctl.readAllow")
	}
	defer func() { _ = file.Close() }()
	return readAllow(file)
}

type allowLineError int

func (e allowLineError) Error() string {
	return fmt.Sprintf("%s:%d: want file:line:col<TAB>MUTATOR<TAB>reason", mutantsAllow, int(e))
}

func readAllow(r io.Reader) (map[string]bool, error) {
	const op = "monacoctl.readAllow"
	allowed := map[string]bool{}
	scanner := bufio.NewScanner(r)
	for n := 1; scanner.Scan(); n++ {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 || strings.TrimSpace(fields[2]) == "" {
			return nil, errs.Wrap(allowLineError(n), errs.CodeDecodeFailed, op)
		}
		allowed[fields[0]+" "+fields[1]] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	return allowed, nil
}

func toolMutation(_ toolEnv) tool {
	gremlinsBin, _ := filepath.Abs("../../.bin/gremlins")
	return mutationTool(mutationEnv{
		moduleDir: ".",
		goBin:     "go",
		gitBin:    "git",
		gremlins:  gremlinsBin,
		tmpDir:    os.TempDir(),
		exec:      runCommand,
		now:       time.Now,
	})
}
