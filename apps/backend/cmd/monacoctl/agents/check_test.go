package agents

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

type reply struct {
	prefix string
	out    string
	err    error
	took   time.Duration
}

type checkHarness struct {
	*fixture
	clock       time.Time
	work        string
	calls       []string
	affected    string
	affectedErr error
	replies     []reply
}

func newCheckHarness(t *testing.T) *checkHarness {
	t.Helper()
	f := newFixtureFrom(t, rootedRepo)
	git(t, f.dir, "update-ref", "refs/remotes/origin/fb", "fb")
	work, err := filepath.EvalSymlinks(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	h := &checkHarness{fixture: f, clock: f.now, work: work}
	f.run = h.run
	return h
}

func (h *checkHarness) run(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
	line := filepath.Base(name) + " " + strings.Join(args, " ")
	if len(args) > 1 && args[0] == "ci" && args[1] == "affected" {
		line = strings.Join(args, " ")
	} else if name == "git" {
		for _, r := range h.replies {
			if strings.HasPrefix(line, r.prefix) {
				return []byte(r.out), r.err
			}
		}
		return Exec(ctx, dir, stdin, name, args...)
	}
	rel, _ := filepath.Rel(h.work, dir)
	h.calls = append(h.calls, rel+": "+line)
	if strings.HasPrefix(line, "ci affected") {
		return []byte(h.affected), h.affectedErr
	}
	for _, r := range h.replies {
		if strings.HasPrefix(line, r.prefix) {
			h.clock = h.clock.Add(r.took)
			return []byte(r.out), r.err
		}
	}
	h.clock = h.clock.Add(time.Second)
	return nil, nil
}

func (h *checkHarness) check(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runCLI(
		context.Background(), h.env, h.dir, h.cached(h.run), append([]string{"check"}, args...), &stdout, &stderr,
		func() time.Time { return h.clock },
	)
	return code, stdout.String(), stderr.String()
}

func (h *checkHarness) base(t *testing.T, files map[string]string) {
	t.Helper()
	h.commit(t, files)
	git(t, h.dir, "branch", "-f", "fb")
	git(t, h.dir, "update-ref", "refs/remotes/origin/fb", "fb")
}

func (h *checkHarness) commit(t *testing.T, files map[string]string) string {
	t.Helper()
	for name, body := range files {
		writeFile(t, filepath.Join(h.dir, name), body)
	}
	git(t, h.dir, "add", "-A")
	git(t, h.dir, "commit", "-q", "-m", "change")
	out, err := Exec(context.Background(), h.dir, "", "git", "rev-parse", "HEAD^{tree}")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func (h *checkHarness) stateDir(t *testing.T, sub string) string {
	t.Helper()
	return filepath.Join(h.Env(t).Common, "pstack", "ms", sub)
}

func TestCheck_runsTheCheapRowForEachChangedPathAndRecordsTheTree(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.base(t, map[string]string{
		"scripts/untouched_test.go":        "func TestUntouched(t *testing.T) {}\n",
		"scripts/testdata/fixture_test.go": `"foo.sh"` + "\nfunc TestFixture(t *testing.T) {}\n",
		"scripts/ci/reads_test.go":         `var s = "foo.sh"` + "\nfunc TestReadsFoo(t *testing.T) {}\n",
		"scripts/test_tool.py":             `open("tool.py")` + "\n",
		"scripts/test_unrelated.py":        "pass\n",
	})
	tree := h.commit(t, map[string]string{
		"apps/backend/internal/x/x.go":           "package x\n",
		"apps/backend/internal/t/t_test.go":      "package t\n",
		"scripts/foo.sh":                         "echo hi\n",
		"scripts/hook":                           "#!/usr/bin/env bash\necho hi\n",
		"scripts/tool.py":                        "print(1)\n",
		"scripts/notes":                          "#!/usr/bin/env python3\n",
		"scripts/own_test.go":                    "func TestOwnA(t *testing.T) {}\nfunc TestOwnB(t *testing.T) {}\n",
		"scripts/test_new.py":                    "pass\n",
		"packages/mobile-core/Sources/A/a.swift": "let a = 1\n",
		"README.md":                              "hi\n",
	})
	h.affected = "./internal/x\n./internal/t\n./cmd/api\n"
	h.replies = []reply{{prefix: "go test -tags faultpoints", took: 3 * time.Second, out: strings.Join([]string{
		`{"Time":"2026-09-27T12:00:01Z","Action":"start","Package":"github.com/monaco/monaco/apps/backend/internal/x"}`,
		`{"Action":"output","Package":"github.com/monaco/monaco/apps/backend/internal/x","Test":"TestX","Output":"=== RUN   TestX\n"}`,
		`{"Action":"pass","Package":"github.com/monaco/monaco/apps/backend/internal/x","Elapsed":1.5,"Output":"ok  \tx\t1.5s\n"}`,
	}, "\n")}}

	code, stdout, stderr := h.check(t)
	if code != 0 {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	want := []string{
		"apps/backend: ci affected --base origin/fb",
		"apps/backend: go build -tags faultpoints ./internal/x ./cmd/api",
		"apps/backend: go vet -tags faultpoints ./internal/x ./internal/t ./cmd/api",
		"apps/backend: go test -tags faultpoints -short -count=1 -p " + strconv.Itoa(max(2, runtime.NumCPU())) +
			" -json ./internal/x ./internal/t ./cmd/api",
		".: bash -n scripts/foo.sh",
		".: bash -n scripts/hook",
		".: shellcheck scripts/foo.sh scripts/hook",
		"scripts: go test -short -count=1 -run ^(TestOwnA|TestOwnB)$ ./",
		"scripts: go test -short -count=1 -run ^(TestReadsFoo)$ ./ci",
		".: python3 -m unittest scripts/test_new.py scripts/test_tool.py",
		"packages/mobile-core: swift test",
	}
	if got := strings.Join(h.calls, "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", got, strings.Join(want, "\n"))
	}
	if n := strings.Count(stdout, "\n"); n > maxLines {
		t.Fatalf("printed %d lines, want at most %d:\n%s", n, maxLines, stdout)
	}
	if !strings.Contains(stdout, "go test -short  ok    3.0s") || !strings.Contains(stdout, "recorded ") {
		t.Fatalf("stdout: %s", stdout)
	}
	record, err := os.ReadFile(filepath.Join(h.stateDir(t, "checks"), tree))
	if err != nil || !strings.HasPrefix(string(record), "head "+h.head(t)+"\nbase origin/fb\n") {
		t.Fatalf("record: %q %v", record, err)
	}
	log, err := os.ReadFile(filepath.Join(h.stateDir(t, "logs"), "check-"+tree[:12]+".log"))
	if err != nil || !strings.Contains(string(log), "=== RUN   TestX\nok  \tx\t1.5s\n") ||
		!strings.Contains(string(log), "&& swift test)") {
		t.Fatalf("log: %q %v", log, err)
	}

	h.calls = nil
	code, stdout, _ = h.check(t)
	if code != 0 || stdout != "stage 0 already passed on tree "+tree[:12]+"\n" || len(h.calls) != 0 {
		t.Fatalf("rerun on a checked tree: %d %q %v", code, stdout, h.calls)
	}
}

func TestCheck_overBudgetExitsOneNamingTheSlowestPackageAndRecordsNothing(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	tree := h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.affected = "./internal/a\n./internal/slow\n"
	h.replies = []reply{{
		prefix: "go test", took: 75 * time.Second, err: errors.New("signal: killed"),
		out: strings.Join([]string{
			`{"Time":"2026-09-27T12:00:02Z","Action":"start","Package":"github.com/monaco/monaco/apps/backend/internal/a"}`,
			`{"Time":"2026-09-27T12:00:03Z","Action":"start","Package":"github.com/monaco/monaco/apps/backend/internal/slow"}`,
			`{"Action":"pass","Package":"github.com/monaco/monaco/apps/backend/internal/a","Elapsed":4}`,
			`{"Action":"skip","Package":"example.com/elsewhere","Elapsed":0}`,
			"not json",
		}, "\n"),
	}}

	code, stdout, stderr := h.check(t, "--base", "fb")
	want := "go test -short row over the 1m0s go budget after 75s; slowest: ./internal/slow (74.0s)"
	if code != 1 || !strings.Contains(stderr, want) {
		t.Fatalf("over budget: %d %q %q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "go test -short  over budget") {
		t.Fatalf("stdout: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(h.stateDir(t, "checks"), tree)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an over-budget run recorded the tree: %v", err)
	}
	log, _ := os.ReadFile(filepath.Join(h.stateDir(t, "logs"), "check-"+tree[:12]+".log"))
	if !strings.Contains(string(log), "not json\nsignal: killed\n") {
		t.Fatalf("log: %q", log)
	}
}

func TestCheck_aFailingRowStopsTheRunWithAnExcerpt(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n", "x.sh": "echo\n"})
	h.affected = "./internal/a\n"
	h.replies = []reply{{prefix: "go vet", err: errors.New("exit status 1: internal/a/a.go:3:1: unreachable code")}}
	code, stdout, stderr := h.check(t)
	if code != 1 || !strings.Contains(stderr, "go vet failed; see the log") ||
		!strings.Contains(
			stdout,
			"go vet          FAIL  go vet -tags faultpoints ./internal/a\n    exit status 1: internal/a/a.go:3:1",
		) {
		t.Fatalf("vet: %d %q %q", code, stdout, stderr)
	}
	if strings.Contains(strings.Join(h.calls, "\n"), "go test") {
		t.Fatalf("ran past the failing row: %v", h.calls)
	}

	h.affected = ""
	noise := make([]string, 0, 12)
	for i := range 12 {
		noise = append(noise, fmt.Sprintf("line %d", i))
	}
	h.replies = []reply{{prefix: "bash -n", out: strings.Join(noise, "\n"), err: errors.New("exit status 2")}}
	code, stdout, _ = h.check(t)
	if code != 1 || strings.Contains(stdout, "line 4\n") || !strings.Contains(stdout, "line 5\n") ||
		!strings.Contains(stdout, "exit status 2\n") {
		t.Fatalf("excerpt keeps the last %d lines: %d %q", excerptLines, code, stdout)
	}
}

func TestCheck_refusesBadInputAndReportsItsOwnFailures(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	code, _, stderr := h.check(t, "--base")
	if code != 2 || !strings.Contains(stderr, "usage: monacoctl agents check [--base <ref>]") {
		t.Fatalf("usage: %d %q", code, stderr)
	}
	tree := h.commit(t, map[string]string{"apps/backend/a.go": "package a\n"})
	writeFile(t, filepath.Join(h.dir, "apps/backend/a.go"), "package b\n")
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "commit first") {
		t.Fatalf("dirty: %d %q", code, stderr)
	}
	git(t, h.dir, "checkout", "--", ".")
	if code, _, stderr := h.check(t, "--base", "nope"); code != 1 || !strings.Contains(stderr, "diff against nope") {
		t.Fatalf("base: %d %q", code, stderr)
	}

	for name, r := range map[string]reply{
		"read HEAD":             {prefix: "git rev-parse HEAD^{tree}", err: errors.New("x")},
		"read the working tree": {prefix: "git status", err: errors.New("x")},
	} {
		h.replies = []reply{r}
		if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, name) {
			t.Fatalf("%s: %d %q", name, code, stderr)
		}
	}
	h.replies = nil
	h.affectedErr = errors.New("go list broke")
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "find affected packages") {
		t.Fatalf("affected: %d %q", code, stderr)
	}
	h.affectedErr = nil

	logs, checks := h.stateDir(t, "logs"), h.stateDir(t, "checks")
	if err := os.RemoveAll(logs); err != nil {
		t.Fatal(err)
	}
	writeFile(t, logs, "")
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "write "+logs) {
		t.Fatalf("log dir: %d %q", code, stderr)
	}
	if err := os.Remove(logs); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(logs, "check-"+tree[:12]+".log"), 0o750); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "write "+logs) {
		t.Fatalf("log file: %d %q", code, stderr)
	}
	if err := os.RemoveAll(logs); err != nil {
		t.Fatal(err)
	}
	writeFile(t, checks, "")
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "write "+checks) {
		t.Fatalf("checks dir: %d %q", code, stderr)
	}
	if err := os.Remove(checks); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(checks, 0o750); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(checks, tree)
	if err := os.Symlink(filepath.Join(t.TempDir(), "gone", "record"), record); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "write "+record) {
		t.Fatalf("record: %d %q", code, stderr)
	}
	if err := os.Remove(record); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := h.check(t); code != 0 || strings.Contains(stdout, "  go") {
		t.Fatalf("no affected packages runs no go rows: %d %q %q", code, stdout, stderr)
	}
}

func TestCheck_shellShebangNeedsAnExtensionlessReadableFile(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	env := h.Env(t)
	writeFile(t, filepath.Join(h.dir, "run.py"), "#!/bin/bash\n")
	writeFile(t, filepath.Join(h.dir, "plain"), "#!/bin/zsh\n")
	if err := os.Symlink(filepath.Join(h.dir, "missing"), filepath.Join(h.dir, "dangling")); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]bool{"run.py": false, "plain": true, "dangling": false} {
		if got := env.hasShellShebang(file); got != want {
			t.Errorf("%s: got %v want %v", file, got, want)
		}
	}
}

func TestCheck_eachRowHasItsOwnBudgetAndTheRunHasNone(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{
		"apps/backend/internal/a/a.go":           "package a\n",
		"packages/mobile-core/Sources/A/a.swift": "let a = 1\n",
	})
	h.affected = "./internal/a\n"
	h.replies = []reply{{prefix: "go test", took: 50 * time.Second}, {prefix: "swift test", took: 34 * time.Second}}
	if code, stdout, stderr := h.check(t); code != 0 || !strings.Contains(stdout, "swift test      ok    34.0s") {
		t.Fatalf("a 50 s go row and a 34 s swift row pass: %d %q %q", code, stdout, stderr)
	}

	h.commit(t, map[string]string{"packages/mobile-core/Sources/A/a.swift": "let a = 2\n"})
	h.replies = []reply{{prefix: "swift test", took: 61 * time.Second, err: errors.New("signal: killed")}}
	code, stdout, stderr := h.check(t)
	if code != 1 || !strings.Contains(stdout, "swift test      over budget") ||
		!strings.Contains(stderr, "swift test row over the 1m0s swift budget after 61s; slowest: swift test (61.0s)") {
		t.Fatalf("an over-budget swift row names swift: %d %q %q", code, stdout, stderr)
	}

	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a // quiet\n"})
	h.replies = []reply{{prefix: "go test", took: 61 * time.Second}}
	if code, _, stderr := h.check(t); code != 1 ||
		!strings.Contains(stderr, "slowest: go test -short (61.0s)") {
		t.Fatalf("a go test with no package events names the row: %d %q", code, stderr)
	}
}

func TestCheck_theOpenAPISpecAloneRunsTheSwiftRow(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{openAPISpec: "openapi: 3.1.0\n"})
	if code, stdout, stderr := h.check(t); code != 0 ||
		!slices.Contains(h.calls, "packages/mobile-core: swift test") {
		t.Fatalf("openapi.yaml runs swift test: %d %q %q %v", code, stdout, stderr, h.calls)
	}
}

func TestCheck_goTestParallelismSplitsTheCPUsBetweenRunningOwners(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ cpus, running, want int }{{8, 6, 2}, {8, 0, 8}, {16, 2, 8}, {2, 1, 2}, {1, 0, 2}} {
		if got := testParallelism(c.cpus, c.running); got != c.want {
			t.Errorf("testParallelism(%d, %d) = %d, want %d", c.cpus, c.running, got, c.want)
		}
	}

	h := newCheckHarness(t)
	env := h.Env(t)
	for ticket := range 7 {
		state := Running
		if ticket == 6 {
			state = Exited
		}
		if err := env.saveRecord(Record{Ticket: ticket + 1, State: state, Started: h.now}); err != nil {
			t.Fatal(err)
		}
	}
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.affected = "./internal/a\n"
	want := "apps/backend: go test -tags faultpoints -short -count=1 -p " +
		strconv.Itoa(testParallelism(runtime.NumCPU(), 6)) + " -json ./internal/a"
	if code, _, stderr := h.check(t); code != 0 || !slices.Contains(h.calls, want) {
		t.Fatalf("six running owners: %d %q\n%s\nwant %s", code, stderr, strings.Join(h.calls, "\n"), want)
	}

	writeFile(t, env.recordPath(9), "{")
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a // again\n"})
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "decode ") {
		t.Fatalf("a broken record fails the check: %d %q", code, stderr)
	}
}

func TestParseConfig_readsTheCheckBudgetSection(t *testing.T) {
	t.Parallel()
	c, err := parseConfig(strings.NewReader(testConfig + "\n[check.budget]\n# per row\nswift = \"90s\"\n"))
	if err != nil || c.Budget["swift"] != 90*time.Second || c.Budget["go"] != time.Minute ||
		c.Budget["shell"] != 10*time.Second {
		t.Fatalf("budget: %v %v", c.Budget, err)
	}
	for body, want := range map[string]string{
		"[check.other]\n":                 `:10: unknown section [check.other]`,
		"[check.budget]\nrust = \"1s\"\n": `:11: unknown key "check.budget.rust"`,
		"[check.budget]\ngo\n":            ":11: want key = value",
		"[check.budget]\ngo = 60s\n":      ":11: quote: invalid syntax",
		"[check.budget]\ngo = \"soon\"\n": `:11: budget go: want a positive duration such as "60s", got "soon"`,
		"[check.budget]\ngo = \"0s\"\n":   `:11: budget go: want a positive duration`,
	} {
		if _, err := parseConfig(strings.NewReader(testConfig + "\n" + body)); err == nil ||
			!strings.Contains(cliText(err), configPath+want) {
			t.Errorf("%q: %v", body, err)
		}
	}
}
