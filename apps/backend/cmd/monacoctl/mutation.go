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

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	mutationUsage   = "usage: monacoctl mutation [--base main | --all]"
	mutantsAllow    = "mutants.allow"
	testOutputLines = 40
	lived           = "LIVED"
	killed          = "KILLED"
	timedOut        = "TIMED OUT"
)

type mutationEnv struct {
	moduleDir, goBin, gitBin, gremlins, tmpDir string
	exec                                       execFunc
}

type execFunc func(ctx context.Context, dir, name string, args ...string) ([]byte, error)

func runCommand(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		err = fmt.Errorf("%w: %s", err, bytes.TrimSpace(exit.Stderr))
	}
	return out, err
}

type listedPackage struct {
	Dir, ImportPath string
	Deps            []string
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
	base := fs.String("base", "main", "")
	all := fs.Bool("all", false, "")
	if fs.Parse(args) != nil || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, mutationUsage)
		return 2
	}
	ctx := context.Background()
	survivors, err := env.run(ctx, *base, *all, stdout)
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

func (env mutationEnv) run(ctx context.Context, base string, all bool, stdout io.Writer) ([]string, error) {
	const op = "monacoctl.mutation"
	allowed, err := readAllowFile(filepath.Join(env.moduleDir, mutantsAllow))
	if err != nil {
		return nil, err
	}
	exclude, err := os.ReadFile(filepath.Join(env.moduleDir, coverageExclude))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	listed, err := env.goList(ctx)
	if err != nil {
		return nil, err
	}
	var changed []string
	if !all {
		if changed, err = env.changedFiles(ctx, base); err != nil {
			return nil, err
		}
	}
	dirs := affectedPackages(listed, changed, all, strings.Fields(string(exclude)))
	_, _ = fmt.Fprintf(stdout, "mutating %d packages: %s\n", len(dirs), strings.Join(dirs, " "))
	var survivors []string
	for _, dir := range dirs {
		report, err := env.unleash(ctx, dir)
		if err != nil {
			return nil, err
		}
		if mostlyTimedOut(report) {
			return nil, errs.Wrap(timedOutError(dir), errs.CodeInternal, op)
		}
		survivors = append(survivors, survivingMutants(dir, report, allowed)...)
	}
	return survivors, nil
}

func (env mutationEnv) goList(ctx context.Context) ([]listedPackage, error) {
	out, err := env.exec(ctx, env.moduleDir, env.goBin,
		"list", "-f", `{{.Module.Dir}}	{{.Dir}}	{{.ImportPath}}	{{join .Deps " "}}`, "./...")
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "monacoctl.goList")
	}
	var pkgs []listedPackage
	for line := range strings.Lines(string(out)) {
		f := strings.SplitN(strings.TrimRight(line, "\n"), "\t", 4)
		dir := strings.TrimPrefix(strings.TrimPrefix(f[1], f[0]), "/")
		pkgs = append(pkgs, listedPackage{Dir: filepath.ToSlash(dir), ImportPath: f[2], Deps: strings.Fields(f[3])})
	}
	return pkgs, nil
}

func (env mutationEnv) changedFiles(ctx context.Context, base string) ([]string, error) {
	out, err := env.exec(ctx, env.moduleDir, env.gitBin, "diff", "--name-only", "--relative", base+"...HEAD", "--", ".")
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "monacoctl.changedFiles")
	}
	return strings.Fields(string(out)), nil
}

func affectedPackages(pkgs []listedPackage, changed []string, all bool, exclude []string) []string {
	changedDirs := map[string]bool{}
	for _, f := range changed {
		if strings.HasSuffix(f, ".go") {
			changedDirs[path.Dir(f)] = true
		}
	}
	changedPaths := map[string]bool{}
	for _, p := range pkgs {
		if changedDirs[p.Dir] {
			changedPaths[p.ImportPath] = true
		}
	}
	var dirs []string
	for _, p := range pkgs {
		hit := all || changedPaths[p.ImportPath] ||
			slices.ContainsFunc(p.Deps, func(d string) bool { return changedPaths[d] })
		if hit && !excluded(p.Dir+"/", exclude) {
			dirs = append(dirs, p.Dir)
		}
	}
	slices.Sort(dirs)
	return dirs
}

func (env mutationEnv) unleash(ctx context.Context, dir string) (gremlinsReport, error) {
	const op = "monacoctl.unleash"
	file, err := os.CreateTemp(env.tmpDir, "gremlins-*.json")
	if err != nil {
		return gremlinsReport{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	out := file.Name()
	_ = file.Close()
	defer func() { _ = os.Remove(out) }()
	if _, err := env.exec(ctx, env.moduleDir, env.gremlins, "unleash", "--silent", "--timeout-coefficient", "50",
		"--output", out, "--exclude-files", `\.gen\.go$`, "./"+dir); err != nil {
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

type timedOutError string

func (e timedOutError) Error() string {
	return "more mutants in " + string(e) + " timed out than were tested; rerun on a quieter machine"
}

func (env mutationEnv) testOutput(ctx context.Context, dir string) string {
	out, err := env.exec(ctx, env.moduleDir, env.goBin, "test", "-count=1", "./"+dir)
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	lines = lines[max(0, len(lines)-testOutputLines):]
	if err != nil {
		lines = append(lines, err.Error())
	}
	return "go test ./" + dir + " for context:\n" + strings.Join(lines, "\n")
}

func mostlyTimedOut(report gremlinsReport) bool {
	tested, timed := 0, 0
	for _, f := range report.Files {
		for _, m := range f.Mutations {
			switch m.Status {
			case killed, lived:
				tested++
			case timedOut:
				timed++
			}
		}
	}
	return timed > tested
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
