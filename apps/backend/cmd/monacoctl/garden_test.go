package main

import (
	"bytes"
	"context"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/tools/garden"
)

func gardenWorkdir(t *testing.T) string {
	t.Helper()
	wd := t.TempDir()
	for name, body := range map[string]string{
		".golangci.yml":           "version: \"2\"\n",
		".golangci.candidate.yml": "linters:\n  enable: [dupl]\n",
	} {
		if err := os.WriteFile(filepath.Join(wd, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return wd
}

func gardenFake(overrides map[string]garden.Result) garden.Exec {
	results := map[string]garden.Result{
		"git rev-parse": {Stdout: []byte("/repo\napps/backend/\n")},
		"go run":        {Stdout: []byte("[]")},
		"golangci-lint": {Stdout: []byte(`{"Issues":[]}`)},
	}
	maps.Copy(results, overrides)
	return func(_ context.Context, _, name string, args ...string) (garden.Result, error) {
		call := strings.Join(append([]string{filepath.Base(name)}, args...), " ")
		for prefix, res := range results {
			if strings.HasPrefix(call, prefix) {
				return res, nil
			}
		}
		return garden.Result{}, nil
	}
}

func TestGarden_rejectsAnythingButReportWithUsageAndExit2(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"prune"}, {"report", "--bogus"}, {"report", "extra"}} {
		var stderr bytes.Buffer
		if code := toolGarden(toolEnv{wd: t.TempDir()})(
			args,
			io.Discard,
			&stderr,
		); code != 2 ||
			!strings.Contains(stderr.String(), gardenUsage) {
			t.Fatalf("garden %v: exit %d, stderr %q; want 2 and the usage", args, code, stderr.String())
		}
	}
}

func TestGarden_turnsAMutantKeyIntoAFinding(t *testing.T) {
	t.Parallel()
	key := mutantKey("internal/errs", "errs.go", 42, 7, "CONDITIONALS_NEGATION")
	got := mutantFinding(key)
	if got.Rule != "CONDITIONALS_NEGATION" || got.File != "internal/errs/errs.go" || got.Line != 42 {
		t.Fatalf("finding %+v from %q, want CONDITIONALS_NEGATION at internal/errs/errs.go:42", got, key)
	}
}

func TestGarden_writesTheReportAndExitsZeroWhenEveryCheckRan(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		args    []string
		mutants string
	}{
		{[]string{"report"}, "## Surviving mutants (1)\n\n### CONDITIONALS_NEGATION (1)\n\n" +
			"- `apps/backend/a/x.go:3` survived; kill it with a test or list it in mutants.allow with a reason\n"},
		{[]string{"report", "--skip-mutation"}, "## Surviving mutants (skipped)\n\nNot run.\n"},
	} {
		env := gardenEnv{wd: gardenWorkdir(t), exec: gardenFake(nil), mutation: mutationModule(t, "")}
		var stdout, stderr bytes.Buffer
		code := env.run(tc.args, &stdout, &stderr)
		report, err := os.ReadFile(filepath.Join(env.wd, gardenReport))
		if code != 0 || stderr.Len() != 0 || !strings.HasSuffix(stdout.String(), "wrote "+gardenReport+"\n") ||
			err != nil || !strings.Contains(string(report), tc.mutants) {
			t.Fatalf("garden %v: exit %d, stdout %q, stderr %q, read %v; want 0 and a report with %q:\n%s",
				tc.args, code, stdout.String(), stderr.String(), err, tc.mutants, report)
		}
	}
}

func TestGarden_exitsOneAndNamesEachCheckThatCouldNotRun(t *testing.T) {
	t.Parallel()
	env := gardenEnv{
		wd:       gardenWorkdir(t),
		exec:     gardenFake(map[string]garden.Result{"go run": {Code: 1, Stderr: "no module"}}),
		mutation: mutationEnv{moduleDir: filepath.Join(t.TempDir(), "missing")},
	}
	var stdout, stderr bytes.Buffer
	code := env.run([]string{"report"}, &stdout, &stderr)
	lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
	if code != 1 || len(lines) != 2 ||
		lines[0] != "monacoctl garden: Dead code: go run "+garden.Deadcode+" -test -json ./... exited 1: no module" ||
		!strings.HasPrefix(lines[1], "monacoctl garden: Surviving mutants: monacoctl.mutation: internal: open ") {
		t.Fatalf("exit %d, stderr %q; want 1 and one line per check that could not run", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(env.wd, gardenReport)); err != nil {
		t.Fatalf("report not written: %v", err)
	}
}

func TestGarden_exitsOneWhenGitFailsOrTheReportCannotBeWritten(t *testing.T) {
	t.Parallel()
	blocked := gardenWorkdir(t)
	if err := os.Mkdir(filepath.Join(blocked, gardenReport), 0o750); err != nil {
		t.Fatal(err)
	}
	broken := func(context.Context, string, string, ...string) (garden.Result, error) {
		return garden.Result{}, errs.New(errs.CodeInternal, "test.exec")
	}
	for _, tc := range []struct {
		env  gardenEnv
		want string
	}{
		{gardenEnv{wd: gardenWorkdir(t), exec: broken}, "monacoctl garden: garden.run: internal: test.exec: internal\n"},
		{
			gardenEnv{wd: blocked, exec: gardenFake(nil)},
			"monacoctl garden: open " + filepath.Join(blocked, gardenReport) + ": is a directory\n",
		},
	} {
		var stderr bytes.Buffer
		if code := tc.env.run([]string{"report", "--skip-mutation"}, io.Discard, &stderr); code != 1 ||
			stderr.String() != tc.want {
			t.Fatalf("exit %d, stderr %q; want 1 and %q", code, stderr.String(), tc.want)
		}
	}
}

func TestLocalBin_prefersThePinnedFileOverThePath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	wd := filepath.Join(root, "apps", "backend")
	pinned := filepath.Join(wd, "..", "..", ".bin", "sqlc")
	if err := os.MkdirAll(filepath.Join(root, ".bin", "atlas"), 0o750); err != nil {
		t.Fatal(err)
	}
	if got := localBin(wd, "sqlc"); got != "sqlc" {
		t.Fatalf("no pinned sqlc: got %q, want sqlc", got)
	}
	if err := os.WriteFile(pinned, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := localBin(wd, "sqlc"); got != pinned {
		t.Fatalf("pinned sqlc: got %q, want %q", got, pinned)
	}
	if got := localBin(wd, "atlas"); got != "atlas" {
		t.Fatalf("a directory named atlas: got %q, want atlas", got)
	}
}

func TestGardenExec_returnsTheExitCodeAndStderrAndFailsWhenTheToolIsMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	res, err := gardenExec(t.Context(), dir, "sh", "-c", "echo out; echo boom >&2; exit 3")
	if err != nil || string(res.Stdout) != "out\n" || res.Code != 3 || res.Stderr != "exit status 3: boom" {
		t.Fatalf("failing tool: %+v, %v; want stdout, code 3 and stderr", res, err)
	}
	res, err = gardenExec(t.Context(), dir, "sh", "-c", "echo ok")
	if err != nil || string(res.Stdout) != "ok\n" || res.Code != 0 {
		t.Fatalf("passing tool: %+v, %v; want stdout and code 0", res, err)
	}
	if _, err := gardenExec(t.Context(), dir, filepath.Join(dir, "missing")); err == nil {
		t.Fatal("a missing tool returned no error")
	}
}
