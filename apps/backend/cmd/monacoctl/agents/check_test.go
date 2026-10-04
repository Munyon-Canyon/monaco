package agents

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
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
	lint        string
	calls       []string
	affected    string
	affectedErr error
	replies     []reply
	goos        string
	lookPath    func(string) (string, error)
	budget      map[string]time.Duration
	xcodeBuild  func(ctx context.Context, waited string) error
}

func newCheckHarness(t *testing.T) *checkHarness {
	t.Helper()
	f := newFixtureFrom(t, rootedRepo)
	git(t, f.dir, "update-ref", "refs/remotes/origin/fb", "fb")
	work, err := filepath.EvalSymlinks(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	h := &checkHarness{fixture: f, clock: f.now, work: work, lint: "2.14.0\n"}
	f.run = h.run
	h.base(t, map[string]string{"apps/backend/.golangci-lint-version": "v2.14.0\n"})
	return h
}

func (h *checkHarness) run(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
	line := strings.TrimSpace(filepath.Base(name) + " " + strings.Join(args, " "))
	if self, _ := os.Executable(); name == self {
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
	if strings.Contains(line, "xcode-lock.sh") && h.xcodeBuild != nil {
		waited := ""
		for _, a := range args {
			if file, ok := strings.CutPrefix(a, "MONACO_LOCK_WAITED="); ok {
				waited = file
			}
		}
		return nil, h.xcodeBuild(ctx, waited)
	}
	switch line {
	case "golangci-lint version --short":
		return []byte(h.lint), nil
	case "gt parent --no-interactive":
		return nil, nil
	}
	h.clock = h.clock.Add(time.Second)
	return nil, nil
}

func secondsIn(file string) time.Duration {
	raw, _ := os.ReadFile(file)
	var total time.Duration
	for _, f := range strings.Fields(string(raw)) {
		n, _ := strconv.Atoi(f)
		total += time.Duration(n) * time.Second
	}
	return total
}

func (h *checkHarness) check(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runCLI(
		context.Background(), h.env, h.dir, h.cached(h.run), append([]string{"check"}, args...), &stdout, &stderr,
		func(env *Env) {
			env.Now = func() time.Time { return h.clock }
			maps.Copy(env.Config.Budget, h.budget)
			if h.goos != "" {
				env.GOOS = h.goos
			}
			env.Load = func(context.Context, string) (float64, error) { return 41.5, nil }
			env.LookPath = h.lookPath
			if env.LookPath == nil {
				env.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
			}
		},
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

func (h *checkHarness) goTest(t *testing.T, p string, pkgs ...string) []string {
	t.Helper()
	profile := h.profile(t)
	return []string{
		"apps/backend: go test -tags faultpoints -short -count=1 -timeout 20s -p " + p + " -json -coverpkg=" +
			strings.Join(
				slices.DeleteFunc(slices.Clone(pkgs), func(p string) bool { return p == "./internal/t" }),
				",",
			) +
			" -coverprofile=" + profile + " " + strings.Join(pkgs, " "),
		"apps/backend: coverage --profile " + profile,
	}
}

func (h *checkHarness) profile(t *testing.T) string {
	t.Helper()
	return filepath.Join(h.stateDir(t, "coverage"), h.head(t)[:12]+".out")
}

func (h *checkHarness) stateDir(t *testing.T, sub string) string {
	t.Helper()
	return filepath.Join(h.Env(t).Common, "pstack", "ms", sub)
}

func TestAffectedTests_selectsAGlobTheTestFileDeclares(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "tool_manifest_test.go"), ""+
		"package scripts_test\n\n"+
		"func TestToolManifest_everyInvokedBinaryIsInstalled(t *testing.T) {}\n"+
		"func TestToolManifest_plantedUnknownBinaryFails(t *testing.T) {}\n")
	env := &Env{Work: dir}
	want := []string{
		"TestToolManifest_everyInvokedBinaryIsInstalled",
		"TestToolManifest_plantedUnknownBinaryFails",
	}
	for _, changed := range []string{"scripts/foo.sh", "Justfile", ".github/workflows/x.yml"} {
		got, py := env.affectedTests([]string{changed})
		if len(py) != 0 || !slices.Equal(got["."], want) {
			t.Fatalf("%s: tests %#v python %#v", changed, got, py)
		}
	}
	got, py := env.affectedTests([]string{"docs/x.md"})
	if len(got) != 0 || len(py) != 0 {
		t.Fatalf("docs/x.md: tests %#v python %#v", got, py)
	}
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
		"scripts/qa/journey.py":                  "print(1)\n",
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
	pr := ".: env BASE_SHA=origin/fb HEAD_SHA=" + h.head(t) + " PR_LABELS=[] python3 scripts/"
	goTest := h.goTest(t, strconv.Itoa(max(2, runtime.NumCPU())), "./internal/x", "./internal/t", "./cmd/api")
	want := []string{
		".: gt parent --no-interactive",
		"apps/backend: ci affected --base origin/fb",
		"apps/backend: golangci-lint version --short",
		pr + "check-pr-size.py",
		pr + "check-gate-changes.py",
		pr + "check-legacy-growth.py",
		"apps/backend: go build -tags faultpoints ./internal/x ./cmd/api",
		"apps/backend: go vet -tags faultpoints ./internal/x ./internal/t ./cmd/api",
		"apps/backend: golangci-lint run ./internal/x ./internal/t ./cmd/api",
		"apps/backend: go run ./internal/platform/lint/nogo/cmd/nogo ./internal/x ./internal/t ./cmd/api",
		"apps/backend: go run ./cmd/monacoctl lint comments",
		goTest[0],
		goTest[1] + " --only internal/x/x.go",
		".: bash -n scripts/foo.sh",
		".: bash -n scripts/hook",
		".: shellcheck scripts/foo.sh scripts/hook",
		"scripts: go test -short -count=1 -run ^(TestOwnA|TestOwnB)$ ./",
		"scripts: go test -short -count=1 -run ^(TestReadsFoo)$ ./ci",
		".: python3 -m unittest scripts/test_new.py scripts/test_tool.py",
		"packages/mobile-core: swift format lint --strict --recursive --parallel ../../apps/mobile .",
		"packages/mobile-core: swiftlint-ratchet.sh --base origin/fb",
		"packages/mobile-core: mobile-core-test.sh",
		".: python3 scripts/qa/journey.py check",
		".: python3 scripts/qa/test_journey.py",
		".: python3 scripts/qa/test_skill_eval.py",
		".: ready.sh",
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
		!strings.Contains(string(log), "&& ../../scripts/mobile-core-test.sh)") {
		t.Fatalf("log: %q %v", log, err)
	}

	h.calls = nil
	code, stdout, _ = h.check(t)
	if code != 0 || stdout != "stage 0 already passed on tree "+tree[:12]+"\n" || len(h.calls) != 0 {
		t.Fatalf("rerun on a checked tree: %d %q %v", code, stdout, h.calls)
	}
}

func TestJourneyChanged(t *testing.T) {
	t.Parallel()
	for _, file := range []string{
		"apps/mobile/Monaco/App.swift",
		"docs/journeys/auth/sign-in.md",
		"apps/mobile/qa/journeys/accounts.tsv",
		"scripts/qa/journey.py",
	} {
		if !journeyChanged([]string{file}) {
			t.Fatalf("%s did not run the journeys row", file)
		}
	}
	if journeyChanged([]string{"docs/architecture.md"}) {
		t.Fatal("unrelated docs ran the journeys row")
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
	want := "go test -short: package ./internal/slow took 80.0s, over the 20s per-package budget"
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

func TestCheck_aMissingPinnedToolIsTheFirstRowAndStopsTheRun(t *testing.T) {
	t.Parallel()
	for _, tool := range pinnedTools() {
		t.Run(tool, func(t *testing.T) {
			t.Parallel()
			h := newCheckHarness(t)
			tree := h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n", "x.sh": "echo\n"})
			h.affected = "./internal/a\n"
			if err := os.Remove(filepath.Join(h.work, ".bin", tool)); err != nil {
				t.Fatal(err)
			}
			code, stdout, stderr := h.check(t)
			want := "(base origin/fb, parent origin/fb)\n" +
				"  tools           fail  .bin/" + tool + " missing; run scripts/install-" + tool + ".sh\n" +
				"log: "
			if code != 1 || !strings.Contains(stdout, want) || !strings.Contains(stderr, "tools failed; see the log") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			if strings.Contains(stdout, "pr size") || slices.ContainsFunc(h.calls, func(c string) bool {
				return strings.Contains(c, "check-pr-size.py")
			}) {
				t.Fatalf("a row ran after the tools row: %q %v", stdout, h.calls)
			}
			if _, err := os.Stat(filepath.Join(h.stateDir(t, "checks"), tree)); !os.IsNotExist(err) {
				t.Fatalf("recorded a pass: %v", err)
			}
			log, err := os.ReadFile(filepath.Join(h.stateDir(t, "logs"), "check-"+tree[:12]+".log"))
			if err != nil || !strings.Contains(string(log), "fail tools: .bin/"+tool+" missing") {
				t.Fatalf("log: %q %v", log, err)
			}
		})
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

func TestCheck_refusesAGolangciLintThatDiffersFromThePin(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.affected = "./internal/a\n"
	for have, want := range map[string]string{"v2.13.1\n": "v2.13.1", "": "v"} {
		h.lint = have
		code, _, stderr := h.check(t)
		if code != 1 ||
			!strings.Contains(stderr, "golangci-lint on PATH is "+want+" and CI pins v2.14.0; run: go install") {
			t.Fatalf("%q: %d %q", have, code, stderr)
		}
	}
	h.replies = []reply{{prefix: "golangci-lint version", err: errors.New("not found")}}
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "is none and CI pins v2.14.0") {
		t.Fatalf("missing: %d %q", code, stderr)
	}
	if slices.ContainsFunc(h.calls, func(c string) bool { return strings.Contains(c, "go build") }) {
		t.Fatalf("ran a row with the wrong golangci-lint: %v", h.calls)
	}
	git(t, h.dir, "rm", "-q", "apps/backend/.golangci-lint-version")
	git(t, h.dir, "commit", "-q", "-m", "unpin")
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "read the golangci-lint pin") {
		t.Fatalf("no pin: %d %q", code, stderr)
	}
}

func TestCheck_mirroredCommandsStillMatchTheirWorkflows(t *testing.T) {
	t.Parallel()
	for file, steps := range map[string][]string{
		"ci-jobs.yml": {
			"version=$(cat .golangci-lint-version)",
			"args: ./...",
			"go run ./internal/platform/lint/nogo/cmd/nogo ./...",
			"go run ./cmd/monacoctl lint comments",
			"run: scripts/ci/ready.sh",
			"go run ./cmd/monacoctl migrate lint",
			vacuumLint,
			"run: scripts/ci/oasdiff-breaking-test.sh",
			"../../scripts/ci/oasdiff-breaking.sh",
		},
		"pr-format.yml":      {"run: python3 scripts/check-pr-size.py", "run: python3 scripts/check-gate-changes.py"},
		"docs.yml":           {"NO_MKDOCS_2_WARNING: 'true'", "mkdocs build --strict --site-dir site"},
		"ci-mobile-core.yml": {"python3 scripts/check-legacy-growth.py"},
	} {
		body, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", ".github", "workflows", file))
		if err != nil {
			t.Fatal(err)
		}
		for _, step := range steps {
			if !strings.Contains(string(body), step) {
				t.Errorf("%s no longer runs %q; make agents check run what CI runs", file, step)
			}
		}
	}
}

func TestCheck_pathRowsRunTheCIStepsForTheirPathsAgainstTheStackParent(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{openAPISpec: "openapi: 3.1.0\n"})
	git(t, h.dir, "branch", "parent")
	h.commit(t, map[string]string{
		openAPISpec:                           "openapi: 3.1.1\n",
		"apps/backend/migrations/1_a.sql":     "select 1;\n",
		"docs/index.md":                       "hi\n",
		"apps/backend/api/.vacuum.yaml.notes": "x\n",
	})
	h.replies = []reply{{prefix: "gt parent", out: "parent\n"}}
	code, stdout, stderr := h.check(t)
	if code != 0 || !strings.Contains(stdout, "(base origin/fb, parent parent)\n") ||
		!strings.Contains(stdout, "  mkdocs          skip  no .venv/bin/mkdocs here or in the main checkout") {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	spec := filepath.Join(h.stateDir(t, "openapi"), h.head(t)[:12]+".yaml")
	if size := ".: env BASE_SHA=parent HEAD_SHA=" + h.head(
		t,
	) + " PR_LABELS=[] python3 scripts/check-pr-size.py"; !slices.Contains(
		h.calls,
		size,
	) {
		t.Fatalf("size against the stack parent: want %q in\n%s", size, strings.Join(h.calls, "\n"))
	}
	want := []string{
		".: ready.sh",
		"apps/backend: go run ./cmd/monacoctl migrate lint",
		".: docker run --rm -v " + filepath.Join(h.work, "apps", "backend", "api") + ":/api:ro " + vacuumLint,
		".: oasdiff-breaking-test.sh",
		".: oasdiff-breaking.sh " + spec + " " + openAPISpec,
	}
	if got := h.calls[len(h.calls)-len(want):]; !slices.Equal(got, want) {
		t.Fatalf("calls:\n%s\nwant tail:\n%s", strings.Join(h.calls, "\n"), strings.Join(want, "\n"))
	}
	if b, err := os.ReadFile(spec); err != nil || string(b) != "openapi: 3.1.0\n" {
		t.Fatalf("parent spec: %q %v", b, err)
	}

	writeFile(t, filepath.Join(h.dir, ".venv/bin/mkdocs"), "#!/bin/sh\n")
	h.commit(t, map[string]string{"docs/index.md": "hi again\n", "apps/backend/migrations/1_a.sql": "select 2;\n"})
	h.calls, h.replies = nil, []reply{{prefix: "gt parent", err: errors.New("untracked branch")}}
	if code, stdout, stderr := h.check(
		t,
	); code != 0 ||
		!strings.Contains(stdout, "(base origin/fb, parent origin/fb)") {
		t.Fatalf("mkdocs installed: %d %q %q", code, stdout, stderr)
	}
	for _, c := range []string{
		".: ready.sh",
		"apps/backend: go run ./cmd/monacoctl migrate lint",
		".: env NO_MKDOCS_2_WARNING=true " + filepath.Join(h.work, ".venv", "bin", "mkdocs") +
			" build --strict --site-dir site",
	} {
		if !slices.Contains(h.calls, c) {
			t.Errorf("missing %q in\n%s", c, strings.Join(h.calls, "\n"))
		}
	}
}

func TestCheck_prRowsStopAnOversizedDiffAndCountGateWarnings(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"README.md": "hi\n"})
	pr := "env BASE_SHA=origin/fb HEAD_SHA=" + h.head(t) + " PR_LABELS=[] python3 scripts/"
	h.replies = []reply{{
		prefix: pr + "check-gate-changes.py",
		out:    "::warning file=a_test.go,line=3::test-skip\n::warning file=b,line=1::gate-file\n",
	}}
	if code, stdout, stderr := h.check(t); code != 0 ||
		!strings.Contains(stdout, "  gate changes    ok    0.0s  2 warnings in the log\n") ||
		!strings.Contains(stdout, "  pr size         ok    1.0s\n") {
		t.Fatalf("gate warnings: %d %q %q", code, stdout, stderr)
	}

	h.commit(t, map[string]string{"README.md": "hi again\n"})
	pr = "env BASE_SHA=origin/fb HEAD_SHA=" + h.head(t) + " PR_LABELS=[] python3 scripts/"
	h.calls, h.replies = nil, []reply{{
		prefix: pr + "check-pr-size.py", out: "1204 changed lines counted (limit 1000).",
		err: errors.New("exit status 1"),
	}}
	code, stdout, stderr := h.check(t)
	if code != 1 || !strings.Contains(stderr, "pr size failed") || !strings.Contains(stdout, "1204 changed lines") ||
		slices.ContainsFunc(h.calls, func(c string) bool { return strings.Contains(c, "gate") }) {
		t.Fatalf("oversized: %d %q %q %v", code, stdout, stderr, h.calls)
	}
}

func TestCheck_theOpenAPIRowSkipsOasdiffWhenTheParentHasNoSpec(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{openAPISpec: "openapi: 3.1.0\n"})
	h.replies = []reply{{prefix: "gt parent", out: "fb\n"}}
	if code, stdout, stderr := h.check(t); code != 0 || !strings.Contains(stdout, "parent origin/fb)") ||
		!slices.Contains(h.calls, ".: oasdiff-breaking-test.sh") ||
		slices.ContainsFunc(h.calls, func(c string) bool { return strings.HasPrefix(c, ".: oasdiff-breaking.sh") }) {
		t.Fatalf("no parent spec: %d %q %q %v", code, stdout, stderr, h.calls)
	}

	h.commit(t, map[string]string{openAPISpec: "openapi: 3.1.1\n"})
	git(t, h.dir, "update-ref", "refs/remotes/origin/fb", "HEAD~1")
	writeFile(t, h.stateDir(t, "openapi"), "")
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "write "+h.stateDir(t, "openapi")) {
		t.Fatalf("unwritable spec: %d %q", code, stderr)
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
	h.replies = []reply{
		{prefix: "go test", took: 50 * time.Second},
		{prefix: "swift format", took: 0},
		{prefix: "swiftlint-ratchet.sh", took: 0},
		{prefix: "mobile-core-test.sh", took: 34 * time.Second},
	}
	if code, stdout, stderr := h.check(t); code != 0 || !strings.Contains(stdout, "swift test      ok    34.0s") {
		t.Fatalf("a 50 s go row and a 34 s swift row pass: %d %q %q", code, stdout, stderr)
	}

	h.commit(t, map[string]string{"packages/mobile-core/Sources/A/a.swift": "let a = 2\n"})
	h.replies = []reply{
		{prefix: "swift format", took: 0},
		{prefix: "swiftlint-ratchet.sh", took: 0},
		{prefix: "mobile-core-test.sh", took: 151 * time.Second, err: errors.New("signal: killed")},
	}
	code, stdout, stderr := h.check(t)
	wantDetail := "swift test row over the 2m30s swift budget after 151s; slowest: swift test ../../scripts/mobile-core-test.sh (151.0s)"
	if code != 1 || !strings.Contains(stdout, "swift test      over budget") || !strings.Contains(stderr, wantDetail) {
		t.Fatalf("an over-budget swift row names swift: %d %q %q", code, stdout, stderr)
	}

	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a // quiet\n"})
	h.replies = []reply{{prefix: "go build", took: 61 * time.Second}}
	if code, stdout, stderr := h.check(t); code != 1 || !strings.Contains(stdout, "go build        over budget") ||
		!strings.Contains(stderr, "go build row over the 1m0s go budget after 61s; slowest: go build (61.0s)") {
		t.Fatalf("go build still has the go row budget: %d %q %q", code, stdout, stderr)
	}
}

func packageEvents(took ...time.Duration) string {
	lines := make([]string, 0, len(took))
	for i, d := range took {
		lines = append(lines, fmt.Sprintf(
			`{"Action":"pass","Package":"github.com/monaco/monaco/apps/backend/internal/p%d","Elapsed":%g}`,
			i, d.Seconds()))
	}
	return strings.Join(lines, "\n")
}

func TestCheck_theGoTestRowIsBudgetedPerPackageNotPerRow(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.affected = "./internal/a\n"
	many := slices.Repeat([]time.Duration{5 * time.Second}, 35)
	h.replies = []reply{{prefix: "go test", took: 175 * time.Second, out: packageEvents(many...)}}
	if code, stdout, stderr := h.check(t); code != 0 || !strings.Contains(stdout, "go test -short  ok    175.0s") {
		t.Fatalf("35 packages at 5 s each over 175 s pass: %d %q %q", code, stdout, stderr)
	}

	tree := h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a // slow\n"})
	h.replies = []reply{{
		prefix: "go test", took: 30 * time.Second, err: errors.New("exit status 1"),
		out: packageEvents(3*time.Second, 21*time.Second, 20500*time.Millisecond),
	}}
	code, stdout, stderr := h.check(t)
	if code != 1 || !strings.Contains(stdout, "go test -short  over budget") ||
		!strings.Contains(stderr, "go test -short: package ./internal/p1 took 21.0s, over the 20s per-package budget") {
		t.Fatalf("one package over 20 s fails the row naming it: %d %q %q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(h.stateDir(t, "checks"), tree)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an over-budget package recorded the tree: %v", err)
	}

	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a // at the line\n"})
	h.replies = []reply{{prefix: "go test", took: 20 * time.Second, out: packageEvents(20 * time.Second)}}
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "package ./internal/p0 took 20.0s") {
		t.Fatalf("a package at exactly 20 s fails: %d %q", code, stderr)
	}
}

func TestLookPath_usesTheProcessPathWhenUnset(t *testing.T) {
	t.Parallel()
	env := &Env{}
	found, err := env.lookPath("go")
	if err != nil || !strings.Contains(found, "go") {
		t.Fatalf("go: %q %v", found, err)
	}
	_, err = env.lookPath("monacoctl-missing-binary")
	if err == nil || !strings.Contains(err.Error(), "find monacoctl-missing-binary") {
		t.Fatalf("missing: %v", err)
	}
}

func TestCheck_theXcodeRowRunsForAnAppChangeOnDarwinOnly(t *testing.T) {
	t.Parallel()
	found := func(string) (string, error) { return "/usr/bin/xcodebuild", nil }
	build := " MONACO_LOCK_HOLD=300 MONACO_XCODE_LOCK_TIMEOUT=5400 bash -c " + xcodeScript("build-for-testing")
	testCmd := " MONACO_LOCK_HOLD=300 MONACO_XCODE_LOCK_TIMEOUT=5400 bash -c " +
		xcodeScript("-only-testing:MonacoTests test-without-building")
	ran := func(h *checkHarness, cmd string) bool {
		return slices.ContainsFunc(h.calls, func(c string) bool {
			return strings.HasPrefix(c, ".: env MONACO_LOCK_WAITED=") && strings.HasSuffix(c, cmd)
		})
	}
	change := map[string]string{"apps/mobile/Monaco/A.swift": "let a = 1\n"}

	h := newCheckHarness(t)
	h.goos = "darwin"
	h.lookPath = found
	h.commit(t, change)
	code, stdout, stderr := h.check(t)
	if code != 0 || !ran(h, build) || !ran(h, testCmd) ||
		!strings.Contains(stdout, "xcode           ok") {
		t.Fatalf("darwin app change: %d %q %q\n%s", code, stdout, stderr, strings.Join(h.calls, "\n"))
	}
	if !strings.Contains(build, "ensure-ios-privy-config.sh placeholder") ||
		!strings.Contains(build, "-onlyUsePackageVersionsFromResolvedFile") {
		t.Fatalf("build script: %s", build)
	}
	if !slices.Contains(h.calls, ".: install-xcsift.sh") {
		t.Fatalf("missing xcsift install:\n%s", strings.Join(h.calls, "\n"))
	}

	writeFile(t, filepath.Join(h.dir, ".bin", "xcsift"), "#!/bin/sh\n")
	h.commit(t, map[string]string{"apps/mobile/Monaco/B.swift": "let b = 1\n"})
	h.calls = nil
	if code, stdout, stderr = h.check(t); code != 0 ||
		slices.ContainsFunc(h.calls, func(c string) bool { return strings.Contains(c, "install-xcsift") }) ||
		!ran(h, build) {
		t.Fatalf("xcsift already present: %d %q %q\n%s", code, stdout, stderr, strings.Join(h.calls, "\n"))
	}

	h = newCheckHarness(t)
	h.goos = "linux"
	h.lookPath = found
	h.commit(t, change)
	code, stdout, stderr = h.check(t)
	if code != 0 || slices.ContainsFunc(h.calls, func(c string) bool { return strings.Contains(c, "xcodebuild") }) {
		t.Fatalf("linux app change runs xcode: %d %q %q\n%s", code, stdout, stderr, strings.Join(h.calls, "\n"))
	}

	h = newCheckHarness(t)
	h.goos = "darwin"
	h.lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	h.commit(t, change)
	code, stdout, stderr = h.check(t)
	if code != 0 || slices.ContainsFunc(h.calls, func(c string) bool { return strings.Contains(c, "xcodebuild") }) {
		t.Fatalf("darwin without xcodebuild: %d %q %q\n%s", code, stdout, stderr, strings.Join(h.calls, "\n"))
	}
}

func TestCheck_theXcodeRowSkipsTestsTheAppNeverBuilds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		file  string
		xcode bool
		swift bool
	}{
		{"packages/mobile-core/Tests/MonacoCoreTests/ATests.swift", false, true},
		{"packages/mobile-core/Sources/MonacoCore/A.swift", true, true},
		{"apps/mobile/Monaco/A.swift", true, true},
		{"packages/mobile-core/Package.swift", true, true},
		{"packages/mobile-core/Package.resolved", true, true},
		{"packages/flows/README.md", false, true},
	} {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			h := newCheckHarness(t)
			h.goos = "darwin"
			h.lookPath = func(string) (string, error) { return "/usr/bin/xcodebuild", nil }
			h.commit(t, map[string]string{tc.file: "let a = 1\n"})
			code, stdout, stderr := h.check(t)
			ran := func(cmd string) bool {
				return slices.ContainsFunc(h.calls, func(c string) bool { return strings.Contains(c, cmd) })
			}
			xcode, swift := ran("xcodebuild"), ran("mobile-core-test.sh")
			if code != 0 || xcode != tc.xcode || swift != tc.swift {
				t.Fatalf("xcode %v swift %v, want %v %v: %d %q %q\n%s",
					xcode, swift, tc.xcode, tc.swift, code, stdout, stderr, strings.Join(h.calls, "\n"))
			}
		})
	}
}

func TestCheck_theXcodeBudgetStartsWhenTheLockIsTaken(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.goos = "darwin"
	h.lookPath = func(string) (string, error) { return "/usr/bin/xcodebuild", nil }
	h.budget = map[string]time.Duration{"xcode": time.Second}
	writeFile(t, filepath.Join(h.dir, ".bin", "xcsift"), "#!/bin/sh\n")
	h.commit(t, map[string]string{"apps/mobile/Monaco/A.swift": "let a = 1\n"})
	script, err := filepath.Abs("../../../../../scripts/qa/xcode-lock.sh")
	if err != nil {
		t.Fatal(err)
	}
	lockDir := filepath.Join(t.TempDir(), "xcode.lock")
	lockEnv := append(os.Environ(), "MONACO_XCODE_LOCK_DIR="+lockDir, "MONACO_XCODE_SLOTS=1", "MONACO_LOCK_POLL=0.1")
	holder := exec.CommandContext(t.Context(), script, "xcode", "sh", "-c", "echo held; sleep 2")
	holder.Env = lockEnv
	held, err := holder.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = holder.Wait() })
	if _, err := bufio.NewReader(held).ReadString('\n'); err != nil {
		t.Fatalf("the holder never took the lock: %v", err)
	}
	h.xcodeBuild = func(ctx context.Context, waited string) error {
		build := exec.CommandContext(ctx, script, "xcode", "true")
		build.Env = lockEnv
		if waited != "" {
			build.Env = append(slices.Clone(lockEnv), "MONACO_LOCK_WAITED="+waited)
		}
		before := secondsIn(waited)
		runErr := build.Run()
		h.clock = h.clock.Add(secondsIn(waited) - before + 100*time.Millisecond)
		return runErr
	}
	code, stdout, stderr := h.check(t)
	waitedRE := regexp.MustCompile(`xcode +ok +0\.2s  waited ([1-9]\d*)s for xcode lock`)
	if code != 0 || !waitedRE.MatchString(stdout) {
		t.Fatalf("a fast build behind a lock held past the budget passes: %d %q %q", code, stdout, stderr)
	}

	h.commit(t, map[string]string{"apps/mobile/Monaco/A.swift": "let a = 2\n"})
	h.xcodeBuild = func(_ context.Context, waited string) error {
		h.clock = h.clock.Add(5*time.Second + 2*time.Second)
		return os.WriteFile(waited, []byte("5\n"), 0o600)
	}
	code, stdout, stderr = h.check(t)
	if code != 1 || !strings.Contains(stdout, "xcode           over budget") ||
		!strings.Contains(stderr, "xcode row over the 1s xcode budget after 2s") ||
		!strings.Contains(stderr, "waited 5s for xcode lock, not counted") {
		t.Fatalf("a slow build after the lock is taken fails: %d %q %q", code, stdout, stderr)
	}

	h.commit(t, map[string]string{"apps/mobile/Monaco/A.swift": "let a = 3\n"})
	if err := os.RemoveAll(h.stateDir(t, "xcode-lock")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, h.stateDir(t, "xcode-lock"), "not a directory\n")
	if code, stdout, stderr = h.check(t); code != 1 || !strings.Contains(stderr, "xcode-lock") {
		t.Fatalf("an unwritable lock wait file stops the row: %d %q %q", code, stdout, stderr)
	}
}

func TestCheck_theOpenAPISpecAloneRunsTheSwiftRow(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{openAPISpec: "openapi: 3.1.0\n"})
	if code, stdout, stderr := h.check(t); code != 0 ||
		!slices.Contains(h.calls, "packages/mobile-core: mobile-core-test.sh") {
		t.Fatalf("openapi.yaml runs swift test: %d %q %q %v", code, stdout, stderr, h.calls)
	}
}

func TestCheck_aFlowsPackageChangeRunsTheSwiftAndReadyRows(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"packages/flows/app/00.tsv": "id\tscreen\tstatus\tdoc\n"})
	code, stdout, stderr := h.check(t)
	if code != 0 || !slices.Contains(h.calls, "packages/mobile-core: mobile-core-test.sh") ||
		!slices.ContainsFunc(h.calls, func(c string) bool { return strings.Contains(c, "ready.sh") }) {
		t.Fatalf("packages/flows runs swift test and ready: %d %q %q %v", code, stdout, stderr, h.calls)
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
	want := h.goTest(t, strconv.Itoa(testParallelism(runtime.NumCPU(), 6)), "./internal/a")[0]
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
	c, err := parseConfig(strings.NewReader(
		testConfig + "\n[check.budget]\n# per row\nswift = \"90s\"\npackage = \"30s\"\n",
	))
	if err != nil || c.Budget["swift"] != 90*time.Second || c.Budget["go"] != time.Minute ||
		c.Budget["shell"] != 10*time.Second || c.Budget["package"] != 30*time.Second {
		t.Fatalf("budget: %v %v", c.Budget, err)
	}
	for body, want := range map[string]string{
		"[check.other]\n":                 `:11: unknown section [check.other]`,
		"[check.budget]\ngo\n":            ":12: want key = value",
		"[check.budget]\ngo = 60s\n":      ":12: quote: invalid syntax",
		"[check.budget]\ngo = \"soon\"\n": `:12: budget go: want a positive duration such as "60s", got "soon"`,
		"[check.budget]\ngo = \"0s\"\n":   `:12: budget go: want a positive duration`,
	} {
		if _, err := parseConfig(strings.NewReader(testConfig + "\n" + body)); err == nil ||
			!strings.Contains(cliText(err), configPath+want) {
			t.Errorf("%q: %v", body, cliText(err))
		}
	}
	c, err = parseConfig(strings.NewReader(testConfig + "\n[check.budget]\nrust = \"1s\"\n"))
	if err != nil || !slices.Equal(c.Unknown, []string{"check.budget.rust"}) {
		t.Errorf("unknown budget kind: %q %v", c.Unknown, err)
	}
}

func TestCheck_theCoverageRowGatesOnlyTheChangedGoSources(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.base(t, map[string]string{"apps/backend/internal/a/old.go": "package a\n"})
	h.commit(t, map[string]string{"apps/backend/internal/a/a_test.go": "package a\n"})
	h.affected = "./internal/a\n"
	code, stdout, stderr := h.check(t)
	if code != 0 ||
		!strings.Contains(stdout, "  coverage        skip  no Go file outside tests changed under apps/backend") {
		t.Fatalf("test-only change: %d %q %q", code, stdout, stderr)
	}
	if _, err := os.Stat(h.profile(t)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the profile outlived the run: %v", err)
	}

	git(t, h.dir, "rm", "-q", "apps/backend/internal/a/old.go")
	h.commit(t, map[string]string{
		"apps/backend/internal/a/a.go":       "package a\n",
		"apps/backend/internal/a/api.gen.go": "package a\n",
		"scripts/tool.go":                    "package main\n",
	})
	h.calls = nil
	h.replies = []reply{{
		prefix: "coverage --profile",
		out:    "internal/a/a.go:3-5: 2 statements not covered\ncoverage: 50.00% of 4 statements\n",
		err:    errors.New("exit status 1"),
	}}
	code, stdout, stderr = h.check(t)
	want := "apps/backend: coverage --profile " + h.profile(t) +
		" --only internal/a/a.go --only internal/a/api.gen.go"
	if code != 1 || !slices.Contains(h.calls, want) || !strings.Contains(stderr, "coverage failed; see the log") ||
		!strings.Contains(stdout, "    internal/a/a.go:3-5: 2 statements not covered\n") {
		t.Fatalf("uncovered change: %d %q %q\n%s\nwant %s", code, stdout, stderr, strings.Join(h.calls, "\n"), want)
	}

	if err := os.RemoveAll(h.stateDir(t, "coverage")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, h.stateDir(t, "coverage"), "")
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "write "+h.stateDir(t, "coverage")) {
		t.Fatalf("unwritable profile dir: %d %q", code, stderr)
	}
}

func TestCheck_theCoverageRowGatesTheUnchangedSourcesOfAChangedPackage(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.base(t, map[string]string{
		"apps/backend/internal/a/b.go":      "package a\n",
		"apps/backend/internal/a/b_test.go": "package a\n",
		"apps/backend/internal/c/c.go":      "package c\n",
	})
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.affected = "./internal/a\n"
	h.calls = nil
	if code, stdout, stderr := h.check(t); code != 0 {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	want := "apps/backend: coverage --profile " + h.profile(t) + " --only internal/a/a.go --only internal/a/b.go"
	if !slices.Contains(h.calls, want) {
		t.Fatalf("calls: %s\nwant %s", strings.Join(h.calls, "\n"), want)
	}
}

func (h *checkHarness) onPRBranch(t *testing.T, files map[string]string) {
	t.Helper()
	git(t, h.dir, "checkout", "-q", "-b", "pr")
	h.commit(t, files)
}

func (h *checkHarness) moveBase(t *testing.T, files map[string]string) {
	t.Helper()
	git(t, h.dir, "checkout", "-q", "--detach", "fb")
	h.base(t, files)
	git(t, h.dir, "checkout", "-q", "pr")
}

func (h *checkHarness) rebase(t *testing.T) {
	t.Helper()
	git(t, h.dir, "rebase", "-q", "fb")
}

func (h *checkHarness) ranRows() bool {
	return slices.ContainsFunc(h.calls, func(c string) bool { return strings.Contains(c, "check-pr-size.py") })
}

func (h *checkHarness) treeOf(t *testing.T) string {
	t.Helper()
	out, err := Exec(context.Background(), h.dir, "", "git", "rev-parse", "HEAD^{tree}")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestCheck_aRestackThatLeavesTheDiffUnchangedCarriesTheStage0Pass(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.onPRBranch(t, map[string]string{"README.md": "one\n"})
	oldTree := h.treeOf(t)
	if code, stdout, stderr := h.check(t); code != 0 || !h.ranRows() {
		t.Fatalf("first check: %d %q %q %v", code, stdout, stderr, h.calls)
	}
	h.moveBase(t, map[string]string{"NOTES.md": "moved\n"})
	h.rebase(t)
	newTree := h.treeOf(t)
	if newTree == oldTree {
		t.Fatal("the rebase did not change the tree")
	}

	h.calls = nil
	code, stdout, stderr := h.check(t)
	want := "stage 0 carried from tree " + oldTree[:12] + " (same diff against origin/fb)\n"
	if code != 0 || stdout != want || h.ranRows() {
		t.Fatalf("carry: %d %q %q calls %v", code, stdout, stderr, h.calls)
	}
	record, err := os.ReadFile(filepath.Join(h.stateDir(t, "checks"), newTree))
	wantRecord := "head " + h.head(t) + "\nbase origin/fb\ncarried from " + oldTree + "\n"
	if err != nil || string(record) != wantRecord {
		t.Fatalf("record %q, want %q (%v)", record, wantRecord, err)
	}

	h.calls = nil
	if code, stdout, _ := h.check(t, "--base", "origin/fb", "--fresh"); code != 0 || !h.ranRows() ||
		strings.Contains(stdout, "carried") || strings.Contains(stdout, "already") {
		t.Fatalf("fresh on a carried tree: %d %q %v", code, stdout, h.calls)
	}

	h.moveBase(t, map[string]string{"MORE.md": "moved again\n"})
	git(t, h.dir, "reset", "-q", "--hard", "fb")
	h.calls = nil
	if code, stdout, _ := h.check(t); code != 0 || !h.ranRows() || strings.Contains(stdout, "carried") {
		t.Fatalf("empty diff: %d %q %v", code, stdout, h.calls)
	}
	if entries, _ := os.ReadDir(h.stateDir(t, "checks-diff")); len(entries) != 1 {
		t.Fatalf("an empty diff recorded: %v", entries)
	}
	h.commit(t, map[string]string{"README.md": "one\n"})
	checks := h.stateDir(t, "checks")
	if err := os.RemoveAll(checks); err != nil {
		t.Fatal(err)
	}
	writeFile(t, checks, "not a directory\n")
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "write ") {
		t.Fatalf("unwritable carry record: %d %q", code, stderr)
	}
}

func TestCheck_aChangedDiffAfterTheRestackRunsStage0InFull(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.onPRBranch(t, map[string]string{"README.md": "one\n", "gen/out.txt": "generated\n"})
	if code, _, stderr := h.check(t); code != 0 {
		t.Fatalf("first check: %d %q", code, stderr)
	}
	h.moveBase(t, map[string]string{"NOTES.md": "moved\n"})
	h.rebase(t)
	h.commit(t, map[string]string{"README.md": "one \n"})
	h.calls = nil
	if code, stdout, stderr := h.check(t); code != 0 || !h.ranRows() || strings.Contains(stdout, "carried") {
		t.Fatalf("whitespace-only change: %d %q %q %v", code, stdout, stderr, h.calls)
	}
	for name, edit := range map[string]map[string]string{
		"one line edited":          {"README.md": "two\n"},
		"regenerated file differs": {"gen/out.txt": "regenerated\n"},
	} {
		h.commit(t, edit)
		h.calls = nil
		code, stdout, stderr := h.check(t)
		if code != 0 || !h.ranRows() || strings.Contains(stdout, "carried") {
			t.Fatalf("%s: %d %q %q %v", name, code, stdout, stderr, h.calls)
		}
	}

	h.commit(t, map[string]string{"README.md": "three\n"})
	h.replies = []reply{{prefix: "git patch-id", err: errors.New("no patch-id")}}
	h.calls = nil
	if code, stdout, stderr := h.check(t); code != 0 || !h.ranRows() {
		t.Fatalf("patch-id failure: %d %q %q %v", code, stdout, stderr, h.calls)
	}
	h.replies = nil
	h.commit(t, map[string]string{"README.md": "four\n"})
	diffs := h.stateDir(t, "checks-diff")
	if err := os.RemoveAll(diffs); err != nil {
		t.Fatal(err)
	}
	writeFile(t, diffs, "not a directory\n")
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "write ") {
		t.Fatalf("unwritable diff record: %d %q", code, stderr)
	}
}

const rerunPrefix = "go test -tags faultpoints -short -count=1 -timeout 20s -p 1 -json ./internal/slow"

func slowPackageEvents(took time.Duration) string {
	return strings.Join([]string{
		`{"Action":"pass","Package":"github.com/monaco/monaco/apps/backend/internal/fast","Elapsed":1}`,
		fmt.Sprintf(`{"Action":"pass","Package":"github.com/monaco/monaco/apps/backend/internal/slow","Elapsed":%g}`,
			took.Seconds()),
	}, "\n")
}

func rerunEvents(took time.Duration) string {
	return strings.Split(slowPackageEvents(took), "\n")[1]
}

func TestCheck_aPackageOverBudgetPassesWhenRerunAlone(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	tree := h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.affected = "./internal/fast\n./internal/slow\n"
	h.replies = []reply{
		{prefix: rerunPrefix, took: 5 * time.Second, out: rerunEvents(5 * time.Second)},
		{prefix: "go test", took: 26 * time.Second, out: slowPackageEvents(25 * time.Second)},
	}
	code, stdout, stderr := h.check(t)
	if code != 0 {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	want := "go test -short  ok    over budget under load (load1 41.5), ./internal/slow passed alone in 5.0s"
	if !strings.Contains(stdout, want) {
		t.Fatalf("stdout: %s", stdout)
	}
	rerun := slices.IndexFunc(
		h.calls,
		func(c string) bool { return strings.HasPrefix(c, "apps/backend: "+rerunPrefix) },
	)
	if rerun < 0 || strings.Contains(h.calls[rerun], "-cover") {
		t.Fatalf("rerun without coverage flags: %v", h.calls)
	}
	if _, err := os.Stat(filepath.Join(h.stateDir(t, "checks"), tree)); err != nil {
		t.Fatalf("a passing rerun records the tree: %v", err)
	}
}

func TestCheck_aPackageStillOverBudgetWhenRerunAloneFailsNamingTheRerun(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	tree := h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.affected = "./internal/fast\n./internal/slow\n"
	h.replies = []reply{
		{prefix: rerunPrefix, took: 25 * time.Second, out: slowPackageEvents(25 * time.Second)},
		{prefix: "go test", took: 27 * time.Second, out: slowPackageEvents(27 * time.Second)},
	}
	code, stdout, stderr := h.check(t)
	want := "package ./internal/slow took 27.0s, over the 20s per-package budget; rerun alone: package ./internal/slow took 25.0s"
	if code != 1 || !strings.Contains(stderr, want) || !strings.Contains(stdout, "go test -short  over budget") {
		t.Fatalf("still over budget: %d %q %q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(h.stateDir(t, "checks"), tree)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recorded the tree: %v", err)
	}
}

func TestCheck_aRealFailureInAnotherPackageIsNotRerun(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.affected = "./internal/fast\n./internal/slow\n"
	h.replies = []reply{
		{prefix: "go test", took: 26 * time.Second, err: errors.New("exit status 1"), out: strings.Join([]string{
			`{"Action":"fail","Package":"github.com/monaco/monaco/apps/backend/internal/fast","Elapsed":1}`,
			`{"Action":"pass","Package":"github.com/monaco/monaco/apps/backend/internal/slow","Elapsed":25}`,
		}, "\n")},
	}
	if code, _, _ := h.check(
		t,
	); code != 1 ||
		slices.ContainsFunc(h.calls, func(c string) bool { return strings.Contains(c, " -p 1 ") }) {
		t.Fatalf("a failing package must not be masked by a rerun: %d %v", code, h.calls)
	}
}

func TestParseLoad_readsTheFirstFieldOfBothPlatformFormats(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]float64{"{ 41.52 30.10 12.00 }\n": 41.52, "0.75 0.50 0.25 1/300 4242\n": 0.75} {
		if got, err := parseLoad(raw); err != nil || got != want {
			t.Errorf("parseLoad(%q) = %v, %v; want %v", raw, got, err, want)
		}
	}
	if _, err := parseLoad(""); err == nil {
		t.Error("an empty reading is an error")
	}
}

func TestLoadCommand_picksTheReaderForTheOS(t *testing.T) {
	t.Parallel()
	for goos, want := range map[string][]string{
		"darwin": {"sysctl", "-n", "vm.loadavg"},
		"linux":  {"cat", "/proc/loadavg"},
	} {
		if got := loadCommand(goos); !slices.Equal(got, want) {
			t.Errorf("loadCommand(%q) = %v, want %v", goos, got, want)
		}
	}
}

func TestLoadAverage_readsTheHostAndFailsOnACancelledContext(t *testing.T) {
	t.Parallel()
	if got, err := loadAverage(context.Background(), runtime.GOOS); err != nil || got < 0 {
		t.Fatalf("host load: %v, %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := loadAverage(ctx, runtime.GOOS); err == nil {
		t.Fatal("a cancelled context fails the read")
	}
	env := &Env{Load: func(context.Context, string) (float64, error) { return 0, errors.New("no load") }}
	if got := env.load1(context.Background()); got != "unknown" {
		t.Fatalf("load1 = %q, want unknown", got)
	}
	if got := (&Env{GOOS: runtime.GOOS}).load1(context.Background()); got == "unknown" {
		t.Fatalf("the default reader works on %s", runtime.GOOS)
	}
}

func TestCheck_aPackageThatFailsWhenRerunAloneFailsTheRow(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.affected = "./internal/fast\n./internal/slow\n"
	h.replies = []reply{
		{
			prefix: rerunPrefix,
			took:   2 * time.Second,
			err:    errors.New("exit status 1"),
			out:    rerunEvents(2 * time.Second),
		},
		{prefix: "go test", took: 26 * time.Second, out: slowPackageEvents(25 * time.Second)},
	}
	code, stdout, stderr := h.check(t)
	if code != 1 || !strings.Contains(stdout, "go test -short  FAIL  go test") ||
		!strings.Contains(stderr, "go test -short failed; see the log") {
		t.Fatalf("rerun failure: %d %q %q", code, stdout, stderr)
	}
}

func TestCheck_aFlowChangeRunsTheFlowsRowForTheAffectedFlowsOnly(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.base(t, map[string]string{
		flows.Dir + "/00.tsv":                         flows.Header + "\n00\tPing\tsystem\tGET /p\tPing\t\t\tok\tbuilt\tdocs/f.md\n",
		flows.Dir + "/01.tsv":                         flows.Header + "\n01\tSign in\tidentity\tGET /s\tSignIn\t\t\tok\tbuilt\tdocs/f.md\n",
		"apps/backend/internal/modules/system/app.go": "package system\n",
	})
	h.commit(t, map[string]string{"packages/flows/app/00.tsv": "id\tscreen\tstatus\tdoc\n"})
	h.replies = []reply{{prefix: "flows --affected --base origin/fb", out: "00\n"}}
	code, stdout, stderr := h.check(t)
	results := filepath.Join(h.stateDir(t, "flows"), h.head(t)[:12]+".json")
	want := []string{
		"apps/backend: flows --affected --base origin/fb",
		"apps/backend: bash -c go test -tags faultpoints -json -run \"$1\" \"${@:3}\" > \"$2\" || true flows " +
			"^TestFlow(00)_ " + results + " ./internal/modules/system/...",
		"apps/backend: flows check --affected --base origin/fb --from " + results,
		"apps/backend: mobile-core-test.sh --filter (F|Flow)(00)[^a-z0-9]",
	}
	if code != 0 || !strings.Contains(stdout, "  flows  ") {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	for _, c := range want {
		if !slices.Contains(h.calls, c) {
			t.Errorf("missing %q in\n%s", c, strings.Join(h.calls, "\n"))
		}
	}

	h.commit(t, map[string]string{"apps/backend/internal/testkit/flows/f01.go": "package flows\n"})
	h.calls, h.replies = nil, []reply{{prefix: "flows --affected", out: ""}}
	if code, stdout, stderr := h.check(t); code != 0 || !strings.Contains(stdout, "  flows  ") {
		t.Fatalf("no affected flows: %d %q %q", code, stdout, stderr)
	}
	if got := slices.DeleteFunc(
		slices.Clone(h.calls),
		func(c string) bool { return !strings.Contains(c, "flows") },
	); !slices.Equal(
		got,
		[]string{
			"apps/backend: flows --affected --base origin/fb",
			"apps/backend: flows check --affected --base origin/fb",
		},
	) {
		t.Fatalf("no affected flows runs only the structure check: %q", got)
	}
}

func TestCheck_theFlowsRowRunsOnlyForFlowPaths(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		file string
		row  bool
	}{
		{"packages/flows/app/00.tsv", true},
		{flows.Dir + "/00.tsv", true},
		{"apps/backend/internal/testkit/flows/f00.go", true},
		{"packages/mobile-core/Tests/MonacoCoreTests/F00IntegrationTests.swift", true},
		{"packages/mobile-core/Sources/MonacoSystem/Flow00SystemPingModel.swift", true},
		{"packages/mobile-core/Sources/MonacoCore/Format.swift", false},
		{"packages/mobile-core/Package.swift", false},
		{"README.md", false},
	} {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			h := newCheckHarness(t)
			h.base(t, map[string]string{
				"apps/backend/internal/modules/system/app.go": "package system\n",
			})
			h.commit(t, map[string]string{tc.file: "changed\n"})
			code, stdout, stderr := h.check(t)
			if ran := strings.Contains(stdout, "  flows  "); code != 0 || ran != tc.row {
				t.Fatalf("flows row = %v, want %v: %d %q %q", ran, tc.row, code, stdout, stderr)
			}
		})
	}
}

func TestCheck_theFlowsRowReportsWhatItCannotRead(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"packages/flows/app/00.tsv": "id\tscreen\tstatus\tdoc\n"})
	h.replies = []reply{{prefix: "flows --affected", err: errors.New("no git")}}
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "find affected flows: no git") {
		t.Fatalf("a failing flows --affected: %d %q", code, stderr)
	}

	h.replies = []reply{{prefix: "flows --affected", out: "00\n"}}
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "read "+flows.Dir+": no flow files") {
		t.Fatalf("no flow files: %d %q", code, stderr)
	}

	h.commit(
		t,
		map[string]string{
			flows.Dir + "/00.tsv": flows.Header + "\n00\tPing\tsystem\tGET /p\tPing\t\t\tok\tbuilt\tdocs/f.md\n",
		},
	)
	writeFile(t, h.stateDir(t, "flows"), "")
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "write "+h.stateDir(t, "flows")) {
		t.Fatalf("an unwritable state dir: %d %q", code, stderr)
	}
}
