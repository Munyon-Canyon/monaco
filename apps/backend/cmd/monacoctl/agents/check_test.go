package agents

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
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
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

var testDBEnvRE = regexp.MustCompile(`^env TEST_DATABASE_URL=\S+ (CHAOS_SEEDS=\d+ )?`)

const dropDeadClones = `.: docker exec monaco-postgres-test sh -c psql -U "$POSTGRES_USER" -Atc "` + dropDeadRunClones +
	`" | psql -U "$POSTGRES_USER" -q`

func upTestDB(slot int) string {
	if slot == 0 {
		return ".: docker compose -p monaco --profile test up -d --wait postgres-test"
	}
	n := strconv.Itoa(slot)
	return ".: env MONACO_TEST_DB_NAME=monaco-postgres-test-" + n + " MONACO_TEST_DB_PORT=" + strconv.Itoa(54323+slot) +
		" MONACO_TEST_DB_TMPFS=3g docker compose -p monaco-test-" + n + " --profile test up -d --wait postgres-test"
}

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
	names       []string
	affected    string
	excluded    string
	excludedErr error
	affectedErr error
	replies     []reply
	goos        string
	lookPath    func(string) (string, error)
	budget      map[string]time.Duration
	load        float64
	loads       []float64
	loadMu      sync.Mutex
	loadTick    func() (<-chan time.Time, func())
	cores       int
	actions     bool
	landOn      bool
	xcodeBuild  func(ctx context.Context, waited string) error
	swiftWaited []string
	swiftRun    func(waited string)
	onCall      func(line string)
	environ     []string
	laneBuilds  int
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
	if name == "env" && len(args) > 1 && strings.HasPrefix(args[0], "MONACO_LOCK_WAITED=") &&
		strings.HasSuffix(args[1], "mobile-core-test.sh") {
		waited := strings.TrimPrefix(args[0], "MONACO_LOCK_WAITED=")
		h.swiftWaited = append(h.swiftWaited, waited)
		if h.swiftRun != nil {
			h.swiftRun(waited)
		}
		name, args = args[1], args[2:]
	}
	line := strings.TrimSpace(filepath.Base(name) + " " + strings.Join(args, " "))
	lane := filepath.Join(h.work, ".build", "monacoctl-lane")
	if name == "go" && slices.Equal(args, []string{"build", "-o", lane, "./cmd/monacoctl"}) {
		h.laneBuilds++
		for _, r := range h.replies {
			if strings.HasPrefix(line, r.prefix) {
				return []byte(r.out), r.err
			}
		}
		return nil, nil
	}
	if self, _ := os.Executable(); name == self || name == lane {
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
	h.names = append(h.names, name)
	if h.onCall != nil {
		h.onCall(line)
	}
	if strings.HasPrefix(line, "ci affected") {
		if strings.HasSuffix(line, "--excluded") {
			return []byte(h.excluded), errors.Join(h.affectedErr, h.excludedErr)
		}
		return []byte(h.affected), h.affectedErr
	}
	bare := testDBEnvRE.ReplaceAllString(line, "")
	for _, r := range h.replies {
		if strings.HasPrefix(bare, r.prefix) {
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
	case "gt parent --no-interactive", "go env GOPATH", "screens.sh --check":
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
			env.Actions = h.actions
			env.Config.LandOnStage0 = h.landOn
			env.Load = func(context.Context, string) (float64, error) { return h.loadOr(41.5), nil }
			env.Cores = func() int { return cmp.Or(h.cores, 64) }
			env.LoadTick = h.loadTick
			env.LookupEnv = func(key string) (string, bool) {
				for _, kv := range h.environ {
					if k, v, _ := strings.Cut(kv, "="); k == key {
						return v, true
					}
				}
				return "", false
			}
			env.LookPath = h.lookPath
			if env.LookPath == nil {
				env.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
			}
		},
	)
	return code, stdout.String(), stderr.String()
}

func (h *checkHarness) mergeBase(t *testing.T, ref string) string {
	t.Helper()
	out, err := harnessGit(context.Background(), h.dir, "", "merge-base", "HEAD", ref)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func (h *checkHarness) loadOr(def float64) float64 {
	h.loadMu.Lock()
	defer h.loadMu.Unlock()
	if len(h.loads) > 1 {
		v := h.loads[0]
		h.loads = h.loads[1:]
		return v
	}
	if len(h.loads) == 1 {
		return h.loads[0]
	}
	return cmp.Or(h.load, def)
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
	out, err := harnessGit(context.Background(), h.dir, "", "rev-parse", "HEAD^{tree}")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func (h *checkHarness) goTest(t *testing.T, p, parallel, cover string, pkgs ...string) []string {
	t.Helper()
	profile := h.profile(t)
	return []string{
		"apps/backend: " + strings.Join(testDB{}.testEnv(), " ") +
			" go test -tags faultpoints -short -count=1 -timeout 1m20s -p " + p + " -parallel " + parallel +
			" -json -coverpkg=" + cover +
			" -coverprofile=" + profile +
			" " + strings.Join(pkgs, " "),
		"apps/backend: coverage --profile " + profile,
	}
}

func (h *checkHarness) goTestNoCover(p, parallel string, pkgs ...string) string {
	return "apps/backend: " + strings.Join(testDB{}.testEnv(), " ") +
		" go test -tags faultpoints -short -count=1 -timeout 1m20s -p " + p + " -parallel " + parallel +
		" -json " + strings.Join(pkgs, " ")
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

func TestAffectedTests_aChangedScriptsTestFileRunsTheTestsThatScanScriptsTests(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "executable_writes_test.go"), ""+
		"package scripts_test\n\n"+
		"func TestScriptsTests_writeExecutablesThroughForkLockedHelper(t *testing.T) {}\n")
	writeFile(t, filepath.Join(dir, "scripts", "ci_path_filter_test.go"), ""+
		"package scripts_test\n\n"+
		"func TestCIPathFilter_scriptsRunWhenAFileTheyReadChanges(t *testing.T) {}\n")
	env := &Env{Work: dir}
	want := []string{
		"TestCIPathFilter_scriptsRunWhenAFileTheyReadChanges",
		"TestScriptsTests_writeExecutablesThroughForkLockedHelper",
	}
	for _, changed := range []string{"scripts/new_thing_test.go", "scripts/sub/new_thing_test.go"} {
		got, py := env.affectedTests([]string{changed})
		if len(py) != 0 || !slices.Equal(got["."], want) {
			t.Fatalf("%s: tests %#v python %#v", changed, got, py)
		}
	}
}

func TestCheck_aChangeToAFileAScriptsTestReadsRunsThatTest(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		changed string
		want    []string
	}{
		{
			"a file a scripts test reads", "docs/agents/owner.md",
			[]string{"scripts: go test -short -count=1 -run ^(TestBriefs_dispatchPromptStaysUnder400Tokens)$ ./"},
		},
		{"a file no scripts test reads", "docs/agents/verifier.md", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newCheckHarness(t)
			h.base(t, map[string]string{
				"docs/agents/owner.md":    "brief\n",
				"docs/agents/verifier.md": "brief\n",
				"scripts/agent_briefs_test.go": `func TestBriefs_dispatchPromptStaysUnder400Tokens(t *testing.T) {
	_ = readRepo(t, repoRoot(t), "docs/agents/owner.md")
}
`,
			})
			h.commit(t, map[string]string{tc.changed: "changed\n"})
			code, stdout, stderr := h.check(t)
			var got []string
			for _, c := range h.calls {
				if strings.HasPrefix(c, "scripts: go test") {
					got = append(got, c)
				}
			}
			if code != 0 || !slices.Equal(got, tc.want) {
				t.Fatalf("check: %d %q %q\nscripts tests %q, want %q", code, stdout, stderr, got, tc.want)
			}
		})
	}
}

func TestCheck_aDeletedOrRenamedFileRunsTheCitedPathTests(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
	}{
		{"a deleted file", []string{"rm", "-q", "docs/how-to/ship-a-ticket.md"}},
		{"a renamed file", []string{"mv", "docs/how-to/ship-a-ticket.md", "docs/how-to/ship.md"}},
	}
	want := "scripts: go test -short -count=1 -run " +
		"^(TestSkillPaths_everyBacktickedPathInABackendSkillExists|" +
		"TestBackendAgentsMD_staysUnder60LinesAndCitesOnlyRealPaths)$ ./"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newCheckHarness(t)
			h.base(t, map[string]string{"docs/how-to/ship-a-ticket.md": "steps\n"})
			git(t, h.dir, tc.args...)
			git(t, h.dir, "commit", "-q", "-m", "change")
			code, stdout, stderr := h.check(t)
			var got []string
			for _, c := range h.calls {
				if strings.HasPrefix(c, "scripts: go test") {
					got = append(got, c)
				}
			}
			if code != 0 || !slices.Equal(got, []string{want}) {
				t.Fatalf("check: %d %q %q\nscripts tests %q, want %q", code, stdout, stderr, got, want)
			}
		})
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
	}, "\n")}, {prefix: "go list", out: "m m/internal/x |\nm m/internal/t | m/internal/x\nm m/cmd/api m/internal/x |\n"}}

	code, stdout, stderr := h.check(t)
	if code != 0 {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	pr := ".: env BASE_SHA=" + h.mergeBase(t, "origin/fb") + " HEAD_SHA=" + h.head(t) + " PR_LABELS=[] python3 scripts/"
	p := strconv.Itoa(testParallelism(runtime.NumCPU(), 1))
	parallel := strconv.Itoa(testParallelPerPackage(runtime.NumCPU(), 1, testParallelism(runtime.NumCPU(), 1)))
	goTest := h.goTest(t, p, parallel, "./internal/x", "./internal/x", "./internal/t", "./cmd/api")
	want := []string{
		".: gt parent --no-interactive",
		"apps/backend: ci affected --base origin/fb",
		"apps/backend: ci affected --base origin/fb --excluded",
		"apps/backend: go list -f " + importListFormat + " ./...",
		"apps/backend: go env GOPATH",
		"apps/backend: golangci-lint version --short",
		pr + "check-gate-changes.py",
		pr + "check-legacy-growth.py",
		"apps/backend: go build -o /dev/null -tags faultpoints ./internal/x ./cmd/api",
		"apps/backend: go vet -tags faultpoints ./internal/x ./internal/t ./cmd/api",
		"apps/backend: golangci-lint run --allow-parallel-runners ./internal/x ./internal/t ./cmd/api",
		"apps/backend: go run ./internal/platform/lint/nogo/cmd/nogo ./internal/x ./internal/t ./cmd/api",
		"apps/backend: go run ./cmd/monacoctl lint comments",
		upTestDB(0),
		dropDeadClones,
		goTest[0],
		goTest[1] + " --only internal/x/x.go",
		".: bash -n scripts/foo.sh",
		".: bash -n scripts/hook",
		".: shellcheck scripts/foo.sh scripts/hook",
		"scripts: go test -short -count=1 -run ^(TestOwnA|TestOwnB)$ ./",
		"scripts: go test -short -count=1 -run ^(TestReadsFoo)$ ./ci",
		".: python3 -m unittest scripts/test_new.py scripts/test_tool.py",
		"packages/mobile-core: swift format lint --strict --recursive --parallel ../../apps/mobile .",
		"packages/mobile-core: swiftlint-ratchet.sh --base " + h.mergeBase(t, "origin/fb"),
		"packages/mobile-core: mobile-core-test.sh",
		".: screens.sh --check",
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
	record, err := h.Env(t).readCheckRecord(tree)
	if err != nil || record.Head != h.head(t) || record.Base != "origin/fb" ||
		!slices.Contains(record.Rows, "go test -short") {
		t.Fatalf("record: %+v %v", record, err)
	}
	log, err := os.ReadFile(filepath.Join(h.stateDir(t, "logs"), "check-"+tree[:12]+".log"))
	if err != nil || !strings.Contains(string(log), "=== RUN   TestX\nok  \tx\t1.5s\n") ||
		!strings.Contains(string(log), ".swift.waited ../../scripts/mobile-core-test.sh)") {
		t.Fatalf("log: %q %v", log, err)
	}

	h.calls = nil
	code, stdout, stderr = h.check(t)
	if code != 0 || stdout != "stage 0 already passed on tree "+tree[:12]+"\n" || len(h.calls) != 0 {
		t.Fatalf("rerun on a checked tree: %d %q %q %v", code, stdout, stderr, h.calls)
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
	h.replies = []reply{{prefix: "go list", out: "m m/internal/a |\nm m/internal/slow m/internal/a |\n"}, {
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
	want := "go test -short: package ./internal/slow took 81.0s, over the 20s per-package budget"
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

func (h *checkHarness) failureNotes(t *testing.T) []failureNote {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(h.Env(t).Common, ".monaco", "check-failures", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	notes := make([]failureNote, 0, len(paths))
	for _, p := range paths {
		var n failureNote
		body, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &n); err != nil {
			t.Fatal(err)
		}
		notes = append(notes, n)
	}
	return notes
}

func TestCheck_aThirdFailureOnOneRowHandsBackWithExitThree(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.affected = "./internal/a\n"
	h.replies = []reply{{prefix: "go vet", err: errors.New("exit status 1")}}
	for i := 1; i <= 2; i++ {
		code, stdout, _ := h.check(t)
		if code != 1 || strings.Contains(stdout, "in a row") {
			t.Fatalf("failure %d: %d %q", i, code, stdout)
		}
	}
	code, stdout, _ := h.check(t)
	want := "go vet failed 3 checks in a row: push the draft (gt submit --stack --no-interactive --draft) " +
		"and hand back under \"What came up\"; do not run agents check again on this failure"
	if code != 3 || !strings.HasSuffix(stdout, want+"\n") {
		t.Fatalf("third failure: %d %q", code, stdout)
	}
	if notes := h.failureNotes(
		t,
	); len(notes) != 1 || notes[0].Row != "go vet" || notes[0].Count != 3 ||
		notes[0].Tree == "" {
		t.Fatalf("note: %+v", notes)
	}
}

func TestCheck_aFailureOnADifferentRowResetsTheCount(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n", "x.sh": "echo\n"})
	h.affected = "./internal/a\n"
	h.replies = []reply{{prefix: "go vet", err: errors.New("exit status 1")}}
	h.check(t)
	h.check(t)
	h.replies = []reply{{prefix: "bash -n", err: errors.New("exit status 2")}}
	code, stdout, _ := h.check(t)
	if code != 1 || strings.Contains(stdout, "in a row") {
		t.Fatalf("other row: %d %q", code, stdout)
	}
	if notes := h.failureNotes(t); len(notes) != 1 || notes[0].Count != 1 || notes[0].Row == "go vet" {
		t.Fatalf("note: %+v", notes)
	}
}

func TestCheck_aPassDeletesTheFailureNote(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.affected = "./internal/a\n"
	h.replies = []reply{{prefix: "go vet", err: errors.New("exit status 1")}}
	h.check(t)
	h.check(t)
	if len(h.failureNotes(t)) != 1 {
		t.Fatal("no note after failures")
	}
	h.replies = nil
	if code, stdout, _ := h.check(t, "--fresh"); code != 0 {
		t.Fatalf("pass: %d %q", code, stdout)
	}
	if notes := h.failureNotes(t); len(notes) != 0 {
		t.Fatalf("note survived a pass: %+v", notes)
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
			want := "(base origin/fb, parent " + h.mergeBase(t, "origin/fb") + ")\n" +
				"  tools           fail  .bin/" + tool + " missing; run scripts/install-" + tool + ".sh\n" +
				"log: "
			if code != 1 || !strings.Contains(stdout, want) || !strings.Contains(stderr, "tools failed; see the log") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			if strings.Contains(stdout, "gate changes") || slices.ContainsFunc(h.calls, func(c string) bool {
				return strings.Contains(c, "check-gate-changes.py")
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
		"read HEAD":              {prefix: "git rev-parse HEAD^{tree}", err: errors.New("x")},
		"read the working tree":  {prefix: "git status", err: errors.New("x")},
		"diff against origin/fb": {prefix: "git diff --name-only --diff-filter=DR", err: errors.New("x")},
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

func TestCheck_findsGolangciLintInGOPATHWhenItIsNotOnPATH(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.affected = "./internal/a\n"
	gopath := t.TempDir()
	bin := filepath.Join(gopath, "bin", "golangci-lint")
	writeFile(t, bin, "#!/bin/sh\n")
	h.replies = []reply{{prefix: "go env GOPATH", out: gopath + "\n"}}
	if code, stdout, stderr := h.check(t); code != 0 {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	if n := slices.Index(h.names, bin); n < 0 || h.calls[n] != "apps/backend: golangci-lint version --short" {
		t.Fatalf("version probe skipped %s: %v", bin, h.calls)
	}
	if n := slices.Index(h.names[slices.Index(h.names, bin)+1:], bin); n < 0 {
		t.Fatalf("lint run skipped %s: %v", bin, h.calls)
	}

	for i, r := range []reply{
		{prefix: "go env GOPATH", err: errors.New("no go")},
		{prefix: "go env GOPATH", out: t.TempDir() + "\n"},
	} {
		h.replies = []reply{r}
		h.calls, h.names = nil, nil
		src := "package a\n\nconst N = " + strconv.Itoa(i) + "\n"
		h.commit(t, map[string]string{"apps/backend/internal/a/a.go": src})
		if code, stdout, stderr := h.check(t); code != 0 || !slices.Contains(h.names, "golangci-lint") {
			t.Fatalf("no GOPATH binary %v: %d %q %q %v", r, code, stdout, stderr, h.names)
		}
	}

	h.lookPath = func(string) (string, error) { return "/usr/local/bin/golangci-lint", nil }
	h.calls, h.names = nil, nil
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n\nconst N = 9\n"})
	if code, stdout, stderr := h.check(t); code != 0 ||
		slices.Contains(h.names, bin) || slices.Contains(h.calls, "apps/backend: go env GOPATH") {
		t.Fatalf("on PATH: %d %q %q %v", code, stdout, stderr, h.calls)
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
		"pr-format.yml":      {"run: python3 scripts/check-gate-changes.py"},
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
	if code != 0 || !strings.Contains(stdout, "(base origin/fb, parent "+h.mergeBase(t, "parent")+")\n") ||
		!strings.Contains(stdout, "  mkdocs          skip  no .venv/bin/mkdocs here or in the main checkout") {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	spec := filepath.Join(h.stateDir(t, "openapi"), h.head(t)[:12]+".yaml")
	if gate := ".: env BASE_SHA=" + h.mergeBase(t, "parent") + " HEAD_SHA=" + h.head(
		t,
	) + " PR_LABELS=[] python3 scripts/check-gate-changes.py"; !slices.Contains(
		h.calls,
		gate,
	) {
		t.Fatalf("gate changes against the stack parent: want %q in\n%s", gate, strings.Join(h.calls, "\n"))
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
		!strings.Contains(stdout, "(base origin/fb, parent "+h.mergeBase(t, "origin/fb")+")") {
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
	pr := "env BASE_SHA=" + h.mergeBase(t, "origin/fb") + " HEAD_SHA=" + h.head(t) + " PR_LABELS=[] python3 scripts/"
	h.replies = []reply{{
		prefix: pr + "check-gate-changes.py",
		out:    "::warning file=a_test.go,line=3::test-skip\n::warning file=b,line=1::gate-file\n",
	}}
	if code, stdout, stderr := h.check(t); code != 0 ||
		!strings.Contains(stdout, "  gate changes    ok    0.0s  2 warnings in the log\n") {
		t.Fatalf("gate warnings: %d %q %q", code, stdout, stderr)
	}

	h.commit(t, map[string]string{"README.md": "hi again\n"})
	pr = "env BASE_SHA=" + h.mergeBase(t, "origin/fb") + " HEAD_SHA=" + h.head(t) + " PR_LABELS=[] python3 scripts/"
	h.calls, h.replies = nil, []reply{{
		prefix: pr + "check-legacy-growth.py", out: "2 legacy paths grew",
		err: errors.New("exit status 1"),
	}}
	code, stdout, stderr := h.check(t)
	if code != 1 || !strings.Contains(stderr, "legacy growth failed") ||
		!strings.Contains(stdout, "2 legacy paths grew") ||
		slices.ContainsFunc(h.calls, func(c string) bool { return strings.Contains(c, "go build") }) {
		t.Fatalf("legacy growth: %d %q %q %v", code, stdout, stderr, h.calls)
	}
}

func TestCheck_theOpenAPIRowSkipsOasdiffWhenTheParentHasNoSpec(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{openAPISpec: "openapi: 3.1.0\n"})
	h.replies = []reply{{prefix: "gt parent", out: "fb\n"}}
	code, stdout, stderr := h.check(t)
	if code != 0 || !strings.Contains(stdout, "parent "+h.mergeBase(t, "origin/fb")+")") ||
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
	build := " MONACO_LOCK_HOLD=1200 MONACO_XCODE_LOCK_TIMEOUT=5400 bash -c " + xcodeScript("build-for-testing")
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
	if code != 0 || !ran(h, build) || !strings.Contains(stdout, "xcode           ok") {
		t.Fatalf("darwin app change: %d %q %q\n%s", code, stdout, stderr, strings.Join(h.calls, "\n"))
	}
	if slices.ContainsFunc(h.calls, func(c string) bool {
		return strings.Contains(c, "test-without-building") || strings.Contains(c, "MonacoTests") ||
			strings.Contains(c, "xcode-lock.sh sim")
	}) {
		t.Fatalf(
			"stage 0 runs MonacoTests, which belong to the hourly staging ios job:\n%s",
			strings.Join(h.calls, "\n"),
		)
	}
	if !strings.Contains(build, "ensure-ios-privy-config.sh placeholder") ||
		!strings.Contains(build, "scripts/xcode-cache-args.sh") || !strings.Contains(build, `"${cache_args[@]}"`) ||
		strings.Contains(build, "-collect-test-diagnostics") {
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

func TestXcodeScript_shutsDownASimulatorItBooted(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"build-for-testing", "-only-testing:MonacoTests test-without-building"} {
		script := xcodeScript(action)
		resolve := strings.Index(script, "scripts/resolve-ios-sim.sh")
		listBooted := strings.Index(script, "xcrun simctl list devices booted")
		guard := strings.Index(script, `if [[ $booted != *"$udid"* ]]; then trap `)
		trap := strings.Index(script, `trap 'xcrun simctl shutdown "$udid" >/dev/null 2>&1 || true' EXIT`)
		build := strings.Index(script, "scripts/qa/xcode-lock.sh xcodebuild")
		if strings.Count(script, "scripts/resolve-ios-sim.sh") != 1 ||
			!strings.Contains(script, `-destination "platform=iOS Simulator,id=$udid"`) ||
			!strings.Contains(script, "udid=$(scripts/resolve-ios-sim.sh)") {
			t.Fatalf("%s: the udid is not resolved once into a variable:\n%s", action, script)
		}
		if resolve < 0 || listBooted < resolve || guard < listBooted || trap < guard || build < trap {
			t.Fatalf("%s: want resolve, booted list, guarded EXIT trap, then xcodebuild:\n%s", action, script)
		}
		if strings.Contains(script, "simctl erase") || strings.Count(script, "simctl shutdown") != 1 {
			t.Fatalf("%s: unexpected simctl calls:\n%s", action, script)
		}
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
		{"packages/flows/backend/19a.tsv", false, true},
		{"packages/flows/Sources/MonacoFlows/Flow19a.gen.swift", true, true},
		{"packages/flows/Package.swift", true, true},
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
	script := filepath.Join(t.TempDir(), "xcode-lock.sh")
	writeFile(t, script, "#!/bin/sh\nshift\necho 2 >> \"$MONACO_LOCK_WAITED\"\nexec \"$@\"\n")
	h.xcodeBuild = func(ctx context.Context, waited string) error {
		build := exec.CommandContext(ctx, "sh", script, "xcode", "true")
		build.Env = append(os.Environ(), "MONACO_LOCK_WAITED="+waited)
		before := secondsIn(waited)
		runErr := build.Run()
		h.clock = h.clock.Add(secondsIn(waited) - before + 100*time.Millisecond)
		return runErr
	}
	code, stdout, stderr := h.check(t)
	waitedRE := regexp.MustCompile(`xcode +ok +0\.1s  waited ([1-9]\d*)s for xcode lock`)
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

func heldSlots(t *testing.T, env *Env) int {
	t.Helper()
	dir := filepath.Join(env.Common, ".monaco", "test-db")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	busy, err := busySlots(dir)
	if err != nil {
		t.Fatal(err)
	}
	return busy
}

func ticketCount(t *testing.T, env *Env, class string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(env.Common, ".monaco", "check-queue", class))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return len(entries)
}

func TestCheck_theXcodeRowRunsLastHoldingNoTokenAndNoTestDatabase(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.goos = "darwin"
	h.lookPath = func(string) (string, error) { return "/usr/bin/xcodebuild", nil }
	writeFile(t, filepath.Join(h.dir, ".bin", "xcsift"), "#!/bin/sh\n")
	h.commit(t, map[string]string{"apps/mobile/Monaco/A.swift": "let a = 1\n"})
	env := h.Env(t)
	var during []string
	h.xcodeBuild = func(context.Context, string) error {
		if !slices.ContainsFunc(h.calls, func(c string) bool { return strings.Contains(c, "journey.py check") }) {
			during = append(during, "the journeys row had not run")
		}
		for _, class := range []string{"db", "cpu"} {
			if n := ticketCount(t, env, class); n != 0 {
				during = append(during, fmt.Sprintf("%d %s tickets held", n, class))
			}
		}
		if busy := heldSlots(t, env); busy != 0 {
			during = append(during, fmt.Sprintf("%d test database slots held", busy))
		}
		return nil
	}
	code, stdout, stderr := h.check(t)
	if code != 0 || len(during) != 0 || !strings.Contains(stdout, "xcode           ok") {
		t.Fatalf("the xcode row held a token: %d %q %q\n%s", code, during, stdout, stderr)
	}
}

func TestCheck_eachRowHoldsOnlyItsClassTokenAndOnlyDatabaseRowsHoldATestDatabase(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.affected = "./internal/a\n"
	env := h.Env(t)
	seen := map[string]string{}
	h.onCall = func(line string) {
		for _, row := range []string{"go build", "go vet", "go test"} {
			if !strings.Contains(line, row+" ") {
				continue
			}
			seen[row] = fmt.Sprintf("db=%d cpu=%d slots=%d",
				ticketCount(t, env, "db"), ticketCount(t, env, "cpu"), heldSlots(t, env))
		}
	}
	if code, stdout, stderr := h.check(t); code != 0 {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	for row, want := range map[string]string{
		"go build": "db=0 cpu=1 slots=0",
		"go vet":   "db=0 cpu=1 slots=0",
		"go test":  "db=1 cpu=0 slots=1",
	} {
		if seen[row] != want {
			t.Errorf("%s ran holding %q, want %q", row, seen[row], want)
		}
	}
	if ticketCount(t, env, "db") != 0 || ticketCount(t, env, "cpu") != 0 {
		t.Fatal("tickets left after the run")
	}
}

func TestCheck_aRowWithNoClassNeverTouchesTheQueueEvenWhenEveryTokenIsTaken(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"x.sh": "echo\n"})
	env := h.Env(t)
	for _, class := range []string{"db", "cpu"} {
		q := env.checkQueue(class)
		for _, pid := range []int{os.Getpid(), os.Getppid(), 1} {
			writeFile(t, filepath.Join(q.dir, "1-"+strconv.Itoa(pid)), "/other/.worktrees/"+strconv.Itoa(pid)+"\n")
		}
	}
	writeFile(t, filepath.Join(env.Common, localConfigPath), "[check.tokens]\ndb = 1\ncpu = 1\n")
	if code, stdout, stderr := h.check(t); code != 0 || strings.Contains(stdout, "waiting") {
		t.Fatalf("a shell row waited: %d %q %q", code, stdout, stderr)
	}
}

func TestCheck_theSwiftRowDoesNotChargeLockWaitToItsBudget(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"packages/mobile-core/Sources/A/a.swift": "let a = 1\n"})
	h.replies = []reply{
		{prefix: "swift format", took: 0},
		{prefix: "swiftlint-ratchet.sh", took: 0},
		{prefix: "mobile-core-test.sh", took: 200 * time.Second},
	}
	h.swiftRun = func(waited string) {
		if err := os.WriteFile(waited, []byte("100\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	code, stdout, stderr := h.check(t)
	if code != 0 || len(h.swiftWaited) != 1 ||
		!strings.Contains(stdout, "swift test      ok    100.0s  waited 1m40s for swift test lock") {
		t.Fatalf("100 s of a 200 s run behind the lock passes the 150 s budget: %d %q %q %v",
			code, stdout, stderr, h.swiftWaited)
	}

	h.commit(t, map[string]string{"packages/mobile-core/Sources/A/a.swift": "let a = 2\n"})
	h.swiftWaited = nil
	h.replies[2].took = 260 * time.Second
	code, stdout, stderr = h.check(t)
	if code != 1 || !strings.Contains(stdout, "swift test      over budget") ||
		!strings.Contains(stderr, "waited 1m40s for swift test lock, not counted") {
		t.Fatalf("160 s of work after the lock is taken fails: %d %q %q", code, stdout, stderr)
	}
}

func swiftCalls(h *checkHarness) []string {
	var out []string
	for _, c := range h.calls {
		if strings.HasPrefix(c, "packages/mobile-core: ") {
			out = append(out, strings.TrimPrefix(c, "packages/mobile-core: "))
		}
	}
	return out
}

func TestCheck_generatedSwiftAloneRunsOnlyMonacoAPITests(t *testing.T) {
	t.Parallel()
	const header = "generated Swift only; MonacoAPITests, lint skipped"
	for name, files := range map[string]map[string]string{
		"the spec": {openAPISpec: "openapi: 3.1.0\n"},
		"the flows generated files": {
			"packages/flows/Sources/MonacoFlows/Flow19b.gen.swift":                "let a = 1\n",
			"packages/flows/Tests/MonacoFlowsTests/Flow19bOutcomeTests.gen.swift": "let b = 1\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newCheckHarness(t)
			h.commit(t, files)
			code, stdout, stderr := h.check(t)
			if want := []string{"mobile-core-test.sh --filter MonacoAPITests"}; code != 0 ||
				!slices.Equal(swiftCalls(h), want) || !strings.Contains(stdout, header) {
				t.Fatalf("generated Swift runs only the filtered tests: %d %q %q %v", code, stdout, stderr, h.calls)
			}
		})
	}
}

func TestCheck_aHandWrittenSwiftFileAmongGeneratedOnesRunsTheFullSwiftRow(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{
		"packages/mobile-core/Sources/MonacoCore/Foo.swift":   "let a = 1\n",
		"packages/mobile-core/Sources/MonacoAPI/openapi.yaml": "openapi: 3.1.0\n",
	})
	code, stdout, stderr := h.check(t)
	got := swiftCalls(h)
	if code != 0 || len(got) != 3 || !strings.HasPrefix(got[0], "swift format lint") ||
		!strings.HasPrefix(got[1], "swiftlint-ratchet.sh") || got[2] != "mobile-core-test.sh" ||
		strings.Contains(stdout, "lint skipped") {
		t.Fatalf("hand-written Swift runs the full row: %d %q %q %v", code, stdout, stderr, h.calls)
	}
}

func TestGeneratedSwift(t *testing.T) {
	t.Parallel()
	for f, want := range map[string]bool{
		openAPISpec:        true,
		swiftSpecCopy:      true,
		errorCodeCasesFile: true,
		"packages/flows/Sources/MonacoFlows/Flow19b.gen.swift": true,
		"packages/mobile-core/Sources/MonacoCore/Foo.swift":    false,
		"apps/mobile/Monaco/Foo.gen.swift":                     false,
		".swiftlint.yml":                                       false,
	} {
		if got := generatedSwift(f); got != want {
			t.Errorf("generatedSwift(%q) = %v, want %v", f, got, want)
		}
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

func TestTestParallelPerPackage_scalesWithBusySlotsAndPackages(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ cpus, busy, p, want int }{
		{18, 1, 4, 4}, {18, 2, 4, 2}, {18, 3, 4, 2}, {18, 5, 3, 2}, {18, 9, 2, 2}, {64, 1, 4, 8}, {8, 0, 4, 8},
	} {
		if got := testParallelPerPackage(c.cpus, c.busy, c.p); got != c.want {
			t.Errorf("testParallelPerPackage(%d, %d, %d) = %d, want %d", c.cpus, c.busy, c.p, got, c.want)
		}
	}
	if p := testParallelism(18, 1); p != 4 || testParallelPerPackage(18, 1, p) != 4 {
		t.Errorf("narrow closure: p = %d, parallel = %d, want 4 and 4", p, testParallelPerPackage(18, 1, p))
	}
}

func TestCheck_eachBusySlotGetsItsOwnTestDatabaseAndAShareOfTheCPUs(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ cpus, busy, want int }{{18, 4, 4}, {18, 2, 4}, {18, 1, 4}, {8, 0, 4}, {6, 2, 3}, {2, 1, 2}, {1, 0, 2}} {
		if got := testParallelism(c.cpus, c.busy); got != c.want {
			t.Errorf("testParallelism(%d, %d) = %d, want %d", c.cpus, c.busy, got, c.want)
		}
	}

	h := newCheckHarness(t)
	slots := filepath.Join(h.Env(t).Common, ".monaco", "test-db")
	if err := os.MkdirAll(slots, 0o750); err != nil {
		t.Fatal(err)
	}
	for slot := range 3 {
		held, ok, err := tryLock(filepath.Join(slots, strconv.Itoa(slot)))
		if err != nil || !ok {
			t.Fatalf("hold slot %d: %v %v", slot, ok, err)
		}
		t.Cleanup(func() { _ = held.Close() })
	}
	compose := "apps/backend/deployments/compose.yml"
	h.commit(t, map[string]string{
		"apps/backend/internal/a/a.go": "package a\n",
		compose:                        "ports: [\"${MONACO_TEST_DB_PORT:-54323}:5432\"]\n",
	})
	h.affected = "./internal/a\n"
	p := strconv.Itoa(testParallelism(runtime.NumCPU(), 4))
	par := strconv.Itoa(testParallelPerPackage(runtime.NumCPU(), 4, testParallelism(runtime.NumCPU(), 4)))
	goTest := strings.Replace(h.goTest(t, p, par, "./internal/a", "./internal/a")[0], ":54323/", ":54326/", 1)
	drop := ".: docker exec monaco-postgres-test-3 sh -c " + dropLeftoverClones
	if code, _, stderr := h.check(t); code != 0 || !slices.Contains(h.calls, upTestDB(3)) ||
		!slices.Contains(h.calls, drop) || !slices.Contains(h.calls, goTest) {
		t.Fatalf("slot 3: %d %q\n%s\nwant %s\nand %s", code, stderr, strings.Join(h.calls, "\n"), upTestDB(3), goTest)
	}
	busy, err := busySlots(slots)
	for range 1000 {
		if busy == 3 || err != nil {
			break
		}
		runtime.Gosched()
		busy, err = busySlots(slots)
	}
	if busy != 3 || err != nil {
		t.Fatalf("after the check %d slots are busy (%v), want the 3 held", busy, err)
	}

	h.calls = nil
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a // again\n", compose: "ports: []\n"})
	goTest = h.goTest(t, p, par, "./internal/a", "./internal/a")[0]
	if code, _, stderr := h.check(t); code != 0 || !slices.Contains(h.calls, upTestDB(0)) ||
		!slices.Contains(h.calls, dropDeadClones) || !slices.Contains(h.calls, goTest) {
		t.Fatalf(
			"a tree without per-slot containers: %d %q\n%s\nwant %s",
			code,
			stderr,
			strings.Join(h.calls, "\n"),
			goTest,
		)
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
	h.commit(t, map[string]string{"apps/backend/internal/z/z_test.go": "package z\n"})
	h.affected = "./internal/a\n"
	code, stdout, stderr := h.check(t)
	const skipped = "  coverage        skip  no Go source, test or allow file changed under apps/backend"
	if code != 0 || !strings.Contains(stdout, skipped) {
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

func TestCheck_theCoverageRowGatesThePackageSourcesWhenATestOrAllowFileIsRemoved(t *testing.T) {
	t.Parallel()
	for _, gone := range []string{"apps/backend/internal/a/a_test.go", "apps/backend/internal/a/a.allow"} {
		t.Run(filepath.Base(gone), func(t *testing.T) {
			t.Parallel()
			h := newCheckHarness(t)
			h.base(t, map[string]string{
				"apps/backend/internal/a/a.go":      "package a\n",
				"apps/backend/internal/a/a_test.go": "package a\n",
				"apps/backend/internal/a/a.allow":   "# path reason\n",
				"apps/backend/internal/b/b.go":      "package b\n",
				"apps/backend/internal/c/c.go":      "package c\n",
			})
			git(t, h.dir, "rm", "-q", gone)
			h.commit(t, map[string]string{"apps/backend/internal/b/b.go": "package b // changed\n"})
			h.affected = "./internal/a\n"
			listing := "m m/internal/a\nm m/internal/b m/internal/a\nm m/internal/c\n"
			h.replies = []reply{{prefix: "go list", out: listing}}
			h.calls = nil
			if code, stdout, stderr := h.check(t); code != 0 {
				t.Fatalf("check: %d %q %q", code, stdout, stderr)
			}
			want := "apps/backend: coverage --profile " + h.profile(t) +
				" --only internal/a/a.go --only internal/b/b.go"
			if !slices.Contains(h.calls, want) {
				t.Fatalf("calls: %s\nwant %s", strings.Join(h.calls, "\n"), want)
			}
			if !slices.ContainsFunc(h.calls, func(c string) bool {
				return strings.Contains(c, "-coverpkg=./internal/a,./internal/b ") &&
					strings.HasSuffix(c, " ./internal/a ./internal/b")
			}) {
				t.Fatalf("the importer of the package was not tested: %s", strings.Join(h.calls, "\n"))
			}

			h.replies = []reply{{prefix: "go list", err: errors.New("x")}}
			if code, _, stderr := h.check(t, "--fresh"); code != 1 || !strings.Contains(stderr, "list the importers") {
				t.Fatalf("go list failure: %d %q", code, stderr)
			}
		})
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
	return slices.ContainsFunc(h.calls, func(c string) bool { return strings.Contains(c, "check-gate-changes.py") })
}

func (h *checkHarness) treeOf(t *testing.T) string {
	t.Helper()
	out, err := harnessGit(context.Background(), h.dir, "", "rev-parse", "HEAD^{tree}")
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
	want := "stage 0 carried from tree " + oldTree[:12] + " (same diff against " + h.mergeBase(t, "origin/fb") + ")\n"
	if code != 0 || stdout != want || h.ranRows() {
		t.Fatalf("carry: %d %q %q calls %v", code, stdout, stderr, h.calls)
	}
	oldRecord, _ := h.Env(t).readCheckRecord(oldTree)
	record, err := h.Env(t).readCheckRecord(newTree)
	if err != nil || record.Head != h.head(t) || record.Base != "origin/fb" || record.CarriedFrom != oldTree ||
		!slices.Equal(record.Rows, oldRecord.Rows) || len(record.Rows) == 0 {
		t.Fatalf("record %+v, old %+v (%v)", record, oldRecord, err)
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

func TestCheck_aRestackThatOnlyRegeneratesFilesCarriesStage0(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.base(t, map[string]string{
		".gitattributes": "apps/backend/api/openapi.yaml linguist-generated\n" +
			"apps/backend/internal/modules/*/sqlc/*.gen.go\n" +
			"docs/reference/*.md merge=union\n",
	})
	gen := "apps/backend/api/openapi.yaml"
	sqlc := "apps/backend/internal/modules/m/sqlc/q.gen.go"
	h.onPRBranch(t, map[string]string{"README.md": "one\n", gen: "v1\n", sqlc: "v1\n"})
	if code, _, stderr := h.check(t); code != 0 {
		t.Fatalf("first check: %d %q", code, stderr)
	}
	oldTree := h.treeOf(t)

	h.commit(t, map[string]string{gen: "v2\n", sqlc: "v2\n"})
	h.calls = nil
	code, stdout, stderr := h.check(t)
	want := "stage 0 carried from tree " + oldTree[:12] + " (same diff against " + h.mergeBase(t, "origin/fb") + ")\n" +
		"carry key ignored 2 generated files: " + gen + ", " + sqlc + "\n"
	if code != 0 || stdout != want || h.ranRows() {
		t.Fatalf("generated-only change: %d %q %q calls %v", code, stdout, stderr, h.calls)
	}

	h.commit(t, map[string]string{"README.md": "two\n", gen: "v3\n"})
	h.calls = nil
	if code, stdout, stderr := h.check(t); code != 0 || !h.ranRows() || strings.Contains(stdout, "carried") {
		t.Fatalf("hand-written change: %d %q %q %v", code, stdout, stderr, h.calls)
	}

	h.commit(t, map[string]string{"docs/reference/events.md": "not generated here\n"})
	h.calls = nil
	if code, stdout, _ := h.check(t); code != 0 || !h.ranRows() || strings.Contains(stdout, "carried") {
		t.Fatalf("merge=union path is hand-written: %d %q %v", code, stdout, h.calls)
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

func rerunPrefix() string {
	parallel := testParallelPerPackage(runtime.NumCPU(), 1, testParallelism(runtime.NumCPU(), 1))
	return "go test -tags faultpoints -short -count=1 -timeout 20s -p 1 -parallel " + strconv.Itoa(
		parallel,
	) + " -json ./internal/slow"
}

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
		{prefix: rerunPrefix(), took: 5 * time.Second, out: rerunEvents(5 * time.Second)},
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
		func(c string) bool { return strings.Contains(c, " "+rerunPrefix()) },
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
		{prefix: rerunPrefix(), took: 25 * time.Second, out: slowPackageEvents(25 * time.Second)},
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

func (h *checkHarness) loaded(load float64) {
	h.load, h.cores = load, 8
	h.affected = "./internal/fast\n./internal/slow\n"
}

func TestCheck_aPackageWithinTheLoadScaledBudgetPasses(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.loaded(16)
	h.replies = []reply{{prefix: "go test", took: 31 * time.Second, out: slowPackageEvents(30 * time.Second)}}
	code, stdout, stderr := h.check(t)
	want := "ok    31.0s  budget 40s (base 20s, x2.0 for load1 16.0 over 8 cores)"
	if code != 0 || !strings.Contains(stdout, want) {
		t.Fatalf("a package within the scaled budget passes and says why: %d %q %q", code, stdout, stderr)
	}
	if !slices.ContainsFunc(h.calls, func(c string) bool { return strings.Contains(c, " -timeout 1m20s ") }) {
		t.Fatalf("go test gets the scaled timeout: %v", h.calls)
	}
}

func TestCheck_theScaledBudgetIsCappedAtFourTimesTheBase(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.loaded(150)
	h.replies = []reply{
		{prefix: rerunPrefix(), took: 10 * time.Second, out: rerunEvents(10 * time.Second)},
		{prefix: "go test", took: 81 * time.Second, out: slowPackageEvents(81 * time.Second)},
	}
	code, stdout, stderr := h.check(t)
	if code != 0 ||
		!strings.Contains(stdout, "ok    over budget under load (load1 150.0), ./internal/slow passed alone in 10.0s") {
		t.Fatalf("a package over the cap is rerun alone: %d %q %q", code, stdout, stderr)
	}
	if !slices.ContainsFunc(h.calls, func(c string) bool { return strings.Contains(c, " -timeout 1m20s ") }) {
		t.Fatalf("the timeout stops at 4x the base: %v", h.calls)
	}
}

func TestCheck_aNonPackageRowGetsTheLoadScaledBudgetUpToFourTimesTheBase(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/mobile/qa/journeys/a.tsv": "a\n"})
	h.load, h.cores = 32, 8
	h.replies = []reply{{prefix: "python3 scripts/qa/journey.py check", took: 100 * time.Second}}
	code, stdout, stderr := h.check(t)
	want := "journeys        ok    102.0s  budget 2m0s (base 30s, x4.0 for load1 32.0 over 8 cores)"
	if code != 0 || !strings.Contains(stdout, want) {
		t.Fatalf("a journeys row within its scaled budget passes: %d %q %q", code, stdout, stderr)
	}

	h.commit(t, map[string]string{"apps/mobile/qa/journeys/a.tsv": "b\n"})
	h.load = 64
	h.replies = []reply{{prefix: "python3 scripts/qa/journey.py check", took: 130 * time.Second}}
	code, stdout, stderr = h.check(t)
	want = "journeys        ok    over budget under load (load1 64.0, 8 cores), advisory"
	if code != 0 || !strings.Contains(stdout, want) || strings.Contains(stdout, "journeys        over budget") {
		t.Fatalf(
			"a journeys row past 4x its base passes as advisory when the load is past the cap: %d %q %q",
			code,
			stdout,
			stderr,
		)
	}

	h.goos = "darwin"
	h.lookPath = func(string) (string, error) { return "/usr/bin/xcodebuild", nil }
	writeFile(t, filepath.Join(h.dir, ".bin", "xcsift"), "#!/bin/sh\n")
	h.commit(t, map[string]string{"apps/mobile/Monaco/A.swift": "let a = 1\n"})
	h.replies = nil
	h.xcodeBuild = func(context.Context, string) error { return nil }
	if code, stdout, stderr = h.check(t); code != 0 ||
		!slices.ContainsFunc(h.calls, func(c string) bool { return strings.Contains(c, " MONACO_LOCK_HOLD=1200 ") }) {
		t.Fatalf("the xcode lock's hold cap scales with the xcode budget: %d %q %q\n%s",
			code, stdout, stderr, strings.Join(h.calls, "\n"))
	}
}

func TestCheck_aNonPackageOverrunIsAdvisoryOnlyWhenTheCommandPassedLocallyAndLoadIsPastTheCap(t *testing.T) {
	t.Parallel()
	const journeys = "python3 scripts/qa/journey.py check"
	const overrun = "journeys row over the 2m0s (base 30s, x4.0 for load1 64.0 over 8 cores) journeys budget after 130s"
	for _, tc := range []struct {
		name    string
		load    float64
		actions bool
		err     error
		took    time.Duration
		want    string
	}{
		{"a failing command", 64, false, errors.New("boom"), 130 * time.Second, overrun},
		{"CI", 64, true, nil, 130 * time.Second, "journeys row over the 30s journeys budget after 130s"},
		{"under the cap", 20, false, nil, 80 * time.Second, "journeys row over the 1m15s (base 30s, x2.5 for load1 20.0 over 8 cores) journeys budget after 80s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newCheckHarness(t)
			tree := h.commit(t, map[string]string{"apps/mobile/qa/journeys/a.tsv": "a\n"})
			h.load, h.cores, h.actions = tc.load, 8, tc.actions
			h.replies = []reply{{prefix: journeys, took: tc.took, err: tc.err}}
			code, stdout, stderr := h.check(t)
			if code != 1 || !strings.Contains(stderr, tc.want) || strings.Contains(stdout, "advisory") {
				t.Fatalf("the overrun fails: %d %q %q", code, stdout, stderr)
			}
			if _, err := os.Stat(filepath.Join(h.stateDir(t, "checks"), tree)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("recorded the tree: %v", err)
			}
		})
	}
}

func TestCheck_aPackageSlowAloneAndPassingUnderLoadPastTheCapIsAdvisory(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	tree := h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.loaded(150)
	h.replies = []reply{
		{prefix: rerunPrefix(), took: 25 * time.Second, out: slowPackageEvents(25 * time.Second)},
		{prefix: "go test", took: 81 * time.Second, out: slowPackageEvents(81 * time.Second)},
	}
	code, stdout, stderr := h.check(t)
	want := "go test -short  ok    over budget under load (load1 150.0, 8 cores), advisory"
	if code != 0 || !strings.Contains(stdout, want) {
		t.Fatalf("a package slow alone but passing is advisory: %d %q %q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(h.stateDir(t, "checks"), tree)); err != nil {
		t.Fatalf("a passing run records the tree: %v", err)
	}
}

func TestCheck_aRowIsJudgedByTheHighestLoadSeenDuringIt(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/mobile/qa/journeys/a.tsv": "a\n"})
	h.cores = 8
	h.loads = []float64{8, 24}
	h.replies = []reply{{prefix: "python3 scripts/qa/journey.py check", took: 80 * time.Second}}
	code, stdout, stderr := h.check(t)
	want := "journeys        ok    82.0s  budget 1m30s (base 30s, x3.0 for load1 24.0 over 8 cores)"
	if code != 0 || !strings.Contains(stdout, want) {
		t.Fatalf("a row that starts at 1x and ends at 3x passes within 3x: %d %q %q", code, stdout, stderr)
	}

	h.commit(t, map[string]string{"apps/mobile/qa/journeys/a.tsv": "b\n"})
	h.loads = []float64{8, 64}
	h.replies = []reply{{prefix: "python3 scripts/qa/journey.py check", took: 130 * time.Second}}
	code, stdout, stderr = h.check(t)
	want = "journeys        ok    over budget under load (load1 64.0, 8 cores), advisory"
	if code != 0 || strings.Count(stdout, want) != 1 {
		t.Fatalf(
			"a row past 4x its base is judged by the peak load and notes the advisory once: %d %q %q",
			code,
			stdout,
			stderr,
		)
	}
}

func TestCheck_aFailedRowUnderLoadNamesTheHighestLoadSample(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/mobile/qa/journeys/a.tsv": "a\n"})
	h.cores = 8
	h.loads = []float64{8, 24}
	h.replies = []reply{{prefix: "python3 scripts/qa/journey.py check", took: time.Second, err: errors.New("boom")}}
	code, stdout, stderr := h.check(t)
	if code != 1 || !strings.Contains(stdout, "(highest load1 24.0)") {
		t.Fatalf("a failed scaled row prints the highest load: %d %q %q", code, stdout, stderr)
	}
}

func TestCheck_aLoadSampledMidRowRaisesItsBudget(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"packages/mobile-core/Sources/A/a.swift": "let a = 1\n"})
	h.cores = 8
	h.loads = []float64{8}
	ticks := make(chan time.Time)
	h.loadTick = func() (<-chan time.Time, func()) { return ticks, func() {} }
	h.replies = []reply{
		{prefix: "swift format", took: 0},
		{prefix: "swiftlint-ratchet.sh", took: 0},
		{prefix: "mobile-core-test.sh", took: 200 * time.Second},
	}
	h.swiftRun = func(string) {
		h.setLoads(24)
		ticks <- time.Time{}
		ticks <- time.Time{}
		h.setLoads(8)
	}
	code, stdout, stderr := h.check(t)
	want := "swift test      ok    200.0s  budget 7m30s (base 2m30s, x3.0 for load1 24.0 over 8 cores)"
	if code != 0 || !strings.Contains(stdout, want) {
		t.Fatalf("a spike seen only by the minute sample raises the budget: %d %q %q", code, stdout, stderr)
	}
}

func (h *checkHarness) setLoads(v float64) {
	h.loadMu.Lock()
	defer h.loadMu.Unlock()
	h.loads = []float64{v}
}

func TestCheck_aPackageSlowAloneStillFailsUnderLoadNamingBothBudgets(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	tree := h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.loaded(16)
	h.replies = []reply{
		{prefix: rerunPrefix(), took: 25 * time.Second, out: slowPackageEvents(25 * time.Second)},
		{prefix: "go test", took: 46 * time.Second, out: slowPackageEvents(45 * time.Second)},
	}
	code, stdout, stderr := h.check(t)
	want := "package ./internal/slow took 45.0s, over the 40s (base 20s, x2.0 for load1 16.0 over 8 cores) per-package budget; " +
		"rerun alone: package ./internal/slow took 25.0s, over the 20s base budget"
	if code != 1 || !strings.Contains(stderr, want) || !strings.Contains(stdout, "go test -short  over budget") {
		t.Fatalf("slow alone fails: %d %q %q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(h.stateDir(t, "checks"), tree)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recorded the tree: %v", err)
	}
}

func TestCheck_ciKeepsTheBaseBudgetWhateverTheLoad(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.loaded(150)
	h.actions = true
	h.replies = []reply{
		{prefix: rerunPrefix(), took: 25 * time.Second, out: slowPackageEvents(25 * time.Second)},
		{prefix: "go test", took: 31 * time.Second, out: slowPackageEvents(30 * time.Second)},
	}
	code, _, stderr := h.check(t)
	if code != 1 || !strings.Contains(stderr, "took 30.0s, over the 20s per-package budget; rerun alone") {
		t.Fatalf("CI keeps 20s: %d %q", code, stderr)
	}
	if slices.ContainsFunc(h.calls, func(c string) bool {
		return strings.Contains(c, " -timeout ") && !strings.Contains(c, " -timeout 20s ")
	}) {
		t.Fatalf("CI never raises the timeout: %v", h.calls)
	}
}

func TestScaleBudget_followsLoadPerCoreAndNeverBelowTheBase(t *testing.T) {
	t.Parallel()
	base := 20 * time.Second
	for _, tc := range []struct {
		name  string
		env   Env
		limit time.Duration
	}{
		{"idle", Env{Load: fakeLoad(2, nil), Cores: fakeCores(8)}, base},
		{"at one per core", Env{Load: fakeLoad(8, nil), Cores: fakeCores(8)}, base},
		{"busy", Env{Load: fakeLoad(12, nil), Cores: fakeCores(8)}, 30 * time.Second},
		{"capped", Env{Load: fakeLoad(150, nil), Cores: fakeCores(8)}, 80 * time.Second},
		{"unknown load", Env{Load: fakeLoad(0, errors.New("no load")), Cores: fakeCores(8)}, base},
		{"no cores", Env{Load: fakeLoad(150, nil), Cores: fakeCores(0)}, base},
		{"in CI", Env{Load: fakeLoad(150, nil), Cores: fakeCores(8), Actions: true}, base},
	} {
		if got := tc.env.scaleBudget(context.Background(), base).limit; got != tc.limit {
			t.Errorf("%s: limit %s, want %s", tc.name, got, tc.limit)
		}
	}
	env := Env{Load: fakeLoad(150, nil)}
	if got := env.scaleBudget(context.Background(), base); got.cores != runtime.NumCPU() {
		t.Errorf("cores default to the host: %d", got.cores)
	}
}

func fakeLoad(v float64, err error) func(context.Context, string) (float64, error) {
	return func(context.Context, string) (float64, error) { return v, err }
}

func fakeCores(n int) func() int { return func() int { return n } }

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
			prefix: rerunPrefix(),
			took:   2 * time.Second,
			err:    errors.New("exit status 1"),
			out:    rerunEvents(2 * time.Second),
		},
		{prefix: "go test", took: 26 * time.Second, out: slowPackageEvents(25 * time.Second)},
	}
	code, stdout, stderr := h.check(t)
	if code != 1 || !strings.Contains(stdout, "go test -short  FAIL  env TEST_DATABASE_URL=") ||
		!strings.Contains(stderr, "go test -short failed; see the log") {
		t.Fatalf("rerun failure: %d %q %q", code, stdout, stderr)
	}
}

func TestCheck_aFlowChangeRunsTheFlowsRowForTheAffectedFlowsOnly(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.base(t, map[string]string{
		flows.Dir + "/00.tsv":                         flows.Header + "\n00\tPing\tsystem\tGET /p\tPing\t\tsystem.echo;identity.echo\tok\tbuilt\tdocs/f.md\n",
		flows.Dir + "/01.tsv":                         flows.Header + "\n01\tSign in\tidentity\tGET /s\tSignIn\t\t\tok\tbuilt\tdocs/f.md\n",
		"apps/backend/internal/modules/system/app.go": "package system\n",
	})
	h.commit(t, map[string]string{"packages/flows/app/00.tsv": "id\tscreen\tstatus\tdoc\n"})
	h.replies = []reply{{prefix: "flows --affected --base " + h.mergeBase(t, "origin/fb"), out: "00\n"}}
	code, stdout, stderr := h.check(t)
	results := filepath.Join(h.stateDir(t, "flows"), h.head(t)[:12]+".json")
	want := []string{
		"apps/backend: flows --affected --base " + h.mergeBase(t, "origin/fb"),
		"apps/backend: " + strings.Join(testDB{}.testEnv(), " ") +
			" bash -c go test -tags faultpoints -p " + strconv.Itoa(testParallelism(runtime.NumCPU(), 1)) +
			" -parallel " + strconv.Itoa(testParallelPerPackage(runtime.NumCPU(), 1, testParallelism(runtime.NumCPU(), 1))) +
			" -json -run \"$1\" \"${@:3}\" > \"$2\" || true flows " +
			"^TestFlow(00)_ " + results + " ./internal/modules/system/... ./internal/modules/identity/...",
		"apps/backend: flows check --affected --base " + h.mergeBase(t, "origin/fb") + " --from " + results,
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
	if !slices.ContainsFunc(h.swiftWaited, func(f string) bool { return strings.HasSuffix(f, ".flows.waited") }) {
		t.Errorf("the flows swift command carries MONACO_LOCK_WAITED: %v", h.swiftWaited)
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
			"apps/backend: flows --affected --base " + h.mergeBase(t, "origin/fb"),
			"apps/backend: flows check --affected --base " + h.mergeBase(t, "origin/fb"),
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

func TestCheck_coverpkgInstrumentsOnlyChangedPackages(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.base(t, map[string]string{
		"apps/backend/internal/a/a.go":      "package a\n",
		"apps/backend/internal/b/b.go":      "package b\n",
		"apps/backend/internal/c/c_test.go": "package c\n",
		"apps/backend/internal/d/d.go":      "package d\n",
	})
	h.commit(t, map[string]string{
		"apps/backend/internal/a/a.go":      "package a // changed\n",
		"apps/backend/internal/c/c_test.go": "package c // changed\n",
	})
	h.affected = "./internal/a\n./internal/b\n./internal/c\n./internal/d\n"
	h.replies = []reply{
		{
			prefix: "go list",
			out:    "m m/internal/a |\nm m/internal/b m/internal/a |\nm m/internal/c |\nm m/internal/d |\n",
		},
	}
	h.calls = nil
	if code, stdout, stderr := h.check(t); code != 0 {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	p := strconv.Itoa(testParallelism(runtime.NumCPU(), 1))
	parallel := strconv.Itoa(testParallelPerPackage(runtime.NumCPU(), 1, testParallelism(runtime.NumCPU(), 1)))
	covered := h.goTest(t, p, parallel, "./internal/a", "./internal/a", "./internal/b")[0]
	if !slices.Contains(h.calls, covered) {
		t.Fatalf("calls: %s\nwant %s", strings.Join(h.calls, "\n"), covered)
	}
	uncovered := h.goTestNoCover(p, parallel, "./internal/c", "./internal/d")
	if !slices.Contains(h.calls, uncovered) {
		t.Fatalf(
			"the packages that link no instrumented package ran with -cover: %s\nwant %s",
			strings.Join(h.calls, "\n"),
			uncovered,
		)
	}
}

func TestLinkedPackages(t *testing.T) {
	t.Parallel()
	listing := strings.Join([]string{
		"m m/internal/a |",
		"m m/internal/b m/internal/a |",
		"m m/internal/c m/internal/d |",
		"m m/internal/d | m/internal/a",
		"m m/internal/e | m/internal/a",
		"m m/internal/f |",
		"m m/internal/g m/internal/h m/internal/c |",
		"m m/internal/h m/internal/c |",
		"m m |",
	}, "\n")
	tested := []string{
		"./internal/a",
		"./internal/b",
		"./internal/c",
		"./internal/d",
		"./internal/e",
		"./internal/f",
		".",
	}
	linked, unlinked := linkedPackages(listing, tested, []string{"./internal/a"})
	if want := []string{"./internal/a", "./internal/b", "./internal/d", "./internal/e"}; !slices.Equal(linked, want) {
		t.Fatalf(
			"linked %v, want %v (b imports a, d's and e's tests import a, c reaches a only through d's tests)",
			linked,
			want,
		)
	}
	if want := []string{"./internal/c", "./internal/f", "."}; !slices.Equal(unlinked, want) {
		t.Fatalf("unlinked %v, want %v", unlinked, want)
	}
	linked, unlinked = linkedPackages(listing, tested, []string{"."})
	if !slices.Equal(linked, []string{"."}) || len(unlinked) != 6 {
		t.Fatalf("the root package: linked %v unlinked %v", linked, unlinked)
	}
	linked, unlinked = linkedPackages(listing, []string{"./internal/g"}, []string{"./internal/f"})
	if len(linked) != 0 || !slices.Equal(unlinked, []string{"./internal/g"}) {
		t.Fatalf("a diamond that never reaches f: linked %v unlinked %v", linked, unlinked)
	}
}

func TestCoverFlags_nothingToInstrumentWhenNoChangedPackageIsTested(t *testing.T) {
	t.Parallel()
	backend := t.TempDir()
	writeFile(t, filepath.Join(backend, "internal/a/a.go"), "package a\n")
	covered := map[string]bool{"internal/z/z.go": true}
	if got := coverFlags(backend, []string{"./internal/a"}, covered, "p.out"); got != nil {
		t.Fatalf("flags %v, want none", got)
	}
	covered["internal/a/a.go"] = true
	got := coverFlags(backend, []string{"./internal/a"}, covered, "p.out")
	if want := []string{"-coverpkg=./internal/a", "-coverprofile=p.out"}; !slices.Equal(got, want) {
		t.Fatalf("flags %v, want %v", got, want)
	}
}

func TestCheck_coverpkgNoDependencyNoticesAreNotWarnings(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.base(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a // changed\n"})
	h.affected = "./internal/a\n"
	h.replies = []reply{
		{
			prefix: "go test -tags faultpoints",
			out:    "warning: no packages being tested depend on matches for pattern ./internal/a\n",
		},
	}
	code, stdout, stderr := h.check(t)
	if code != 0 || strings.Contains(stdout, "warnings in the log") {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}

	h.replies = []reply{{prefix: "go list", err: errors.New("x")}}
	if code, _, stderr := h.check(t, "--fresh"); code != 1 ||
		!strings.Contains(stderr, "list the imports of the tested packages") {
		t.Fatalf("go list failure: %d %q", code, stderr)
	}
}

func TestCheck_goTestWritesNoProfileWhenNoSourceChanged(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.base(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.commit(t, map[string]string{"apps/backend/internal/z/z_test.go": "package z\n"})
	h.affected = "./internal/a\n"
	h.calls = nil
	if code, stdout, stderr := h.check(t); code != 0 {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	for _, c := range h.calls {
		if strings.Contains(c, "go test") && strings.Contains(c, "-cover") {
			t.Fatalf("go test instruments with no changed source: %s", c)
		}
	}
}

func TestCheck_aBaseThatAdvancedPastTheBranchPointIsNotTheParentTheRowsCompareAgainst(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.base(t, map[string]string{openAPISpec: "openapi: 3.1.0\n"})
	branchPoint := h.head(t)
	h.onPRBranch(t, map[string]string{openAPISpec: "openapi: 3.1.1\n"})
	h.moveBase(t, map[string]string{openAPISpec: "openapi: 9.9.9\n"})
	if tip := h.mergeBase(t, "fb"); tip != branchPoint {
		t.Fatalf("setup: merge base %s, want branch point %s", tip, branchPoint)
	}
	code, stdout, stderr := h.check(t)
	if code != 0 || !strings.Contains(stdout, "parent "+branchPoint+")") {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	legacy := ".: env BASE_SHA=" + branchPoint + " HEAD_SHA=" + h.head(t) +
		" PR_LABELS=[] python3 scripts/check-legacy-growth.py"
	if !slices.Contains(h.calls, legacy) {
		t.Fatalf("legacy growth must see the merge base, want %q in\n%s", legacy, strings.Join(h.calls, "\n"))
	}
	spec := filepath.Join(h.stateDir(t, "openapi"), h.head(t)[:12]+".yaml")
	if b, err := os.ReadFile(spec); err != nil || string(b) != "openapi: 3.1.0\n" {
		t.Fatalf("the openapi row diffs against the spec at the branch point, got %q %v", b, err)
	}
}

func TestCheck_aFailingMergeBaseFallsBackToTheChosenParent(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"README.md": "hi\n"})
	h.replies = []reply{{prefix: "git merge-base", err: errors.New("no merge base")}}
	code, stdout, stderr := h.check(t)
	if code != 0 || !strings.Contains(stdout, "(base origin/fb, parent origin/fb)") {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
}

func TestCheck_aLocalBudgetReplacesTheRepoBudgetAndScalesWithLoad(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	writeFile(t, filepath.Join(h.dir, ".git", localConfigPath), "[check.budget]\njourneys = \"45s\"\n")
	h.commit(t, map[string]string{"apps/mobile/qa/journeys/a.tsv": "a\n"})
	h.load, h.cores = 8, 8
	h.replies = []reply{{prefix: "python3 scripts/qa/journey.py check", took: 40 * time.Second}}
	code, stdout, stderr := h.check(t)
	want := "journeys        ok    42.0s  budget 45s (local)"
	if code != 0 || !strings.Contains(stdout, want) {
		t.Fatalf("40 s passes a local 45 s budget: %d %q %q", code, stdout, stderr)
	}

	h.commit(t, map[string]string{"apps/mobile/qa/journeys/a.tsv": "b\n"})
	h.replies = []reply{{prefix: "python3 scripts/qa/journey.py check", took: 50 * time.Second}}
	code, stdout, stderr = h.check(t)
	want = "journeys row over the 45s (local) journeys budget after 50s"
	if code != 1 || !strings.Contains(stderr, want) {
		t.Fatalf("50 s fails a local 45 s budget: %d %q %q", code, stdout, stderr)
	}

	h.commit(t, map[string]string{"apps/mobile/qa/journeys/a.tsv": "c\n"})
	h.load = 16
	h.replies = []reply{{prefix: "python3 scripts/qa/journey.py check", took: 80 * time.Second}}
	code, stdout, stderr = h.check(t)
	want = "journeys        ok    82.0s  budget 1m30s (base 45s (local), x2.0 for load1 16.0 over 8 cores)"
	if code != 0 || !strings.Contains(stdout, want) {
		t.Fatalf("load scaling applies on top of the local base: %d %q %q", code, stdout, stderr)
	}
}

func TestCheck_withoutALocalFileTheRepoBudgetApplies(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/mobile/qa/journeys/a.tsv": "a\n"})
	h.load, h.cores = 8, 8
	h.replies = []reply{{prefix: "python3 scripts/qa/journey.py check", took: 40 * time.Second}}
	code, stdout, stderr := h.check(t)
	want := "journeys row over the 30s journeys budget after 40s"
	if code != 1 || !strings.Contains(stderr, want) || strings.Contains(stdout+stderr, "(local)") {
		t.Fatalf("40 s fails the repo 30 s budget: %d %q %q", code, stdout, stderr)
	}
}

func TestCheck_keepGoingRunsEveryRowThenNamesEveryFailedOrOverBudgetRow(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	tree := h.commit(t, map[string]string{
		"apps/backend/internal/a/a.go": "package a\n", "x.sh": "echo\n", "apps/mobile/qa/journeys/a.tsv": "a\n",
	})
	h.affected = "./internal/a\n"
	h.load, h.cores = 8, 8
	h.replies = []reply{
		{prefix: "go vet", err: errors.New("exit status 1: unreachable code")},
		{prefix: "python3 scripts/qa/journey.py check", took: 40 * time.Second},
	}
	code, stdout, stderr := h.check(t, "--keep-going")
	for _, want := range []string{"go vet          FAIL", "journeys        over budget", "go test -short  ok", "shellcheck      ok"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q: %q", want, stdout)
		}
	}
	for _, want := range []string{"2 rows failed", "go vet failed; see the log", "journeys row over the 30s journeys budget"} {
		if code != 1 || !strings.Contains(stderr, want) {
			t.Errorf("code=%d stderr lacks %q: %q", code, want, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(h.stateDir(t, "checks"), tree)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a keep-going run with failures recorded the tree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(h.stateDir(t, "logs"), "check-"+tree[:12]+".log")); err != nil {
		t.Fatalf("a keep-going run writes its log: %v", err)
	}
}

func TestCheck_withoutKeepGoingTheFirstFailedRowStopsTheRun(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n", "x.sh": "echo\n"})
	h.affected = "./internal/a\n"
	h.replies = []reply{{prefix: "go vet", err: errors.New("exit status 1")}}
	code, stdout, stderr := h.check(t)
	if code != 1 || strings.Contains(stderr, "rows failed") || strings.Contains(stdout, "bash -n") {
		t.Fatalf("the first failure stops the run: %d %q %q", code, stdout, stderr)
	}
}

func TestCheck_keepGoingWritesTheMarkerOnAFullPass(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	tree := h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n", "x.sh": "echo\n"})
	h.affected = "./internal/a\n"
	code, stdout, stderr := h.check(t, "--keep-going")
	if code != 0 || strings.Contains(stderr, "failed") {
		t.Fatalf("a clean run passes: %d %q %q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(h.stateDir(t, "checks"), tree)); err != nil {
		t.Fatalf("a full pass records the tree: %v", err)
	}
}

func toolchainCheck(t *testing.T, gomod string, environ ...string) (*checkHarness, string) {
	t.Helper()
	h := newCheckHarness(t)
	h.environ = environ
	h.commit(t, map[string]string{
		"apps/backend/go.mod":          gomod,
		"apps/backend/internal/a/a.go": "package a\n",
	})
	h.affected = "./internal/a\n"
	code, stdout, stderr := h.check(t, "--base", "fb")
	if code != 0 {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	return h, stdout
}

func TestCheck_goRowsRunOnTheToolchainGoModPins(t *testing.T) {
	t.Parallel()
	h, stdout := toolchainCheck(t, "module m\n\ngo 1.26.0\n\ntoolchain go1.26.9\n")
	if !strings.Contains(stdout, "go toolchain: go1.26.9 (apps/backend/go.mod)\n") {
		t.Fatalf("header: %q", stdout)
	}
	for _, row := range []string{"go build", "go vet", "go test"} {
		found := false
		for _, call := range h.calls {
			if strings.Contains(call, " go "+strings.TrimPrefix(row, "go ")+" ") {
				found = true
				if !strings.Contains(call, "env GOTOOLCHAIN=go1.26.9 ") {
					t.Errorf("%s ran without the pinned toolchain: %s", row, call)
				}
			}
		}
		if !found {
			t.Errorf("%s did not run: %q", row, h.calls)
		}
	}
}

func TestCheck_aGoModWithoutAToolchainLineLeavesGoToolchainUnset(t *testing.T) {
	t.Parallel()
	h, stdout := toolchainCheck(t, "module m\n\ngo 1.26.0\n")
	if strings.Contains(stdout, "go toolchain:") || strings.Contains(strings.Join(h.calls, "\n"), "GOTOOLCHAIN") {
		t.Fatalf("stdout %q calls %q", stdout, h.calls)
	}
}

func TestCheck_anExplicitGoToolchainIsKept(t *testing.T) {
	t.Parallel()
	h, stdout := toolchainCheck(t, "module m\n\ngo 1.26.0\n\ntoolchain go1.26.9\n", "GOTOOLCHAIN=local")
	if strings.Contains(stdout, "go toolchain:") || strings.Contains(strings.Join(h.calls, "\n"), "GOTOOLCHAIN") {
		t.Fatalf("stdout %q calls %q", stdout, h.calls)
	}
}

func TestCheck_theSubcommandRowsRunTheBinaryBuiltFromThisTree(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.base(t, map[string]string{
		flows.Dir + "/00.tsv":                         flows.Header + "\n00\tPing\tsystem\tGET /p\tPing\t\tsystem.echo\tok\tbuilt\tdocs/f.md\n",
		"apps/backend/internal/modules/system/app.go": "package system\n",
	})
	h.commit(t, map[string]string{
		"packages/flows/app/00.tsv":                 "id\tscreen\tstatus\tdoc\n",
		"apps/backend/internal/modules/system/a.go": "package system\n",
	})
	h.affected = "./internal/modules/system\n"
	h.replies = []reply{{prefix: "flows --affected", out: "00\n"}}
	lane := filepath.Join(h.work, ".build", "monacoctl-lane")
	code, stdout, stderr := h.check(t)
	if code != 0 {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "subcommands: "+lane+" (built from this tree)\n") {
		t.Fatalf("header: %q", stdout)
	}
	if h.laneBuilds != 1 {
		t.Fatalf("the lane binary was built %d times, want once", h.laneBuilds)
	}
	subs := []string{"flows --affected", "flows check", "ci affected", "coverage --profile"}
	ran := map[string]string{}
	for i, call := range h.calls {
		_, line, _ := strings.Cut(call, ": ")
		for _, sub := range subs {
			if strings.HasPrefix(line, sub) {
				ran[sub] = h.names[i]
			}
		}
	}
	for _, sub := range subs {
		if ran[sub] != lane {
			t.Errorf("%s ran %q, want the lane binary %q", sub, ran[sub], lane)
		}
	}
}

func TestCheck_aLaneBuildFailureFailsTheCheckWithItsError(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.replies = []reply{{prefix: "go build -o", out: "a.go:1: syntax error\n", err: errors.New("exit status 1")}}
	code, _, stderr := h.check(t)
	if code != 1 || !strings.Contains(stderr, "build this tree's monacoctl") ||
		!strings.Contains(stderr, "syntax error") {
		t.Fatalf("%d %q", code, stderr)
	}
}

func TestCheck_theGoTestRowHeaderNamesTheClosureAndWhatItExcluded(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.affected = "./internal/a\n./internal/b\n"
	h.excluded = "./cmd/monacoctl\n./cmd/monacoctl/agents\n"
	code, stdout, stderr := h.check(t)
	want := "go test -short  closure of 2 packages; excluded ./cmd/monacoctl ./cmd/monacoctl/agents\n"
	if code != 0 || !strings.Contains(stdout, want) {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	joined := strings.Join(h.calls, "\n")
	if !strings.Contains(joined, "./cmd/monacoctl/agents") ||
		strings.Contains(joined, "go test") && strings.Contains(testCall(h.calls), "cmd/monacoctl") {
		t.Fatalf("build and vet cover the excluded packages, go test does not: %v", h.calls)
	}
}

func testCall(calls []string) string {
	for _, c := range calls {
		if strings.Contains(c, " go test ") || strings.Contains(c, "go test -tags") {
			return c
		}
	}
	return ""
}

func TestCheck_theGoTestRowSkipsWhenEveryAffectedPackageIsExcluded(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.excluded = "./cmd/monacoctl\n"
	code, stdout, stderr := h.check(t)
	if code != 0 ||
		!strings.Contains(stdout, "go test -short  skip  every affected package is excluded: ./cmd/monacoctl\n") {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	if strings.Contains(testCall(h.calls), "go test") {
		t.Fatalf("go test ran with no packages: %v", h.calls)
	}
	h = newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.excludedErr = errors.New("go list broke")
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "find excluded packages") {
		t.Fatalf("excluded: %d %q", code, stderr)
	}
}

func TestCheck_goTestNeverCarriesRace(t *testing.T) {
	t.Parallel()
	for _, on := range []bool{false, true} {
		h := newCheckHarness(t)
		h.landOn = on
		h.base(t, map[string]string{"README.md": "hi\n"})
		h.commit(t, map[string]string{"apps/backend/internal/x/x.go": "package x\n"})
		h.affected = "./internal/x\n"
		if code, stdout, stderr := h.check(t); code != 0 {
			t.Fatalf("on=%v: check: %d %q %q", on, code, stdout, stderr)
		}
		var line string
		for _, c := range h.calls {
			if strings.Contains(c, " go test ") && strings.Contains(c, "-json") {
				line = c
			}
		}
		if line == "" {
			t.Fatalf("on=%v: no go test call in %q", on, h.calls)
		}
		if strings.Contains(line, "-race") {
			t.Fatalf("on=%v: -race in %q", on, line)
		}
	}
}

func TestCheck_aPassingCheckRecordsTheRowsThatRan(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{
		"apps/backend/internal/a/a.go":           "package a\n",
		"packages/mobile-core/Sources/A/a.swift": "let a = 1\n",
	})
	h.affected = "./internal/a\n"
	if code, stdout, stderr := h.check(t); code != 0 {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	record, err := h.Env(t).readCheckRecord(h.treeOf(t))
	if err != nil || !slices.Contains(record.Rows, "swift test") || !slices.Contains(record.Rows, "go build") {
		t.Fatalf("record %+v (%v)", record, err)
	}
}

func TestCheck_anEmptyOrOldRecordReadsAsNoRows(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	env := h.Env(t)
	for name, body := range map[string]string{"empty": "", "old": "head abc\nbase origin/fb\n"} {
		if _, err := env.writeState("checks", name, []byte(body)); err != nil {
			t.Fatal(err)
		}
		record, err := env.readCheckRecord(name)
		if err != nil || len(record.Rows) != 0 {
			t.Fatalf("%s: %+v %v", name, record, err)
		}
	}
}

func TestCheck_workflowsRowLintsAndRunsTheWorkflowTests(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{".github/workflows/staging.yml": "name: staging\n"})
	code, stdout, stderr := h.check(t)
	joined := strings.Join(h.calls, "\n")
	if code != 0 || !strings.Contains(stdout, "workflows       ok") ||
		!strings.Contains(joined, "go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12") ||
		!strings.Contains(joined, "go -C scripts test -short ./ci/ -run TestWorkflowTriggers") ||
		!strings.Contains(joined, "go -C scripts test -short . -run TestCIPathFilter") {
		t.Fatalf("workflow change: %d %q %q\n%s", code, stdout, stderr, joined)
	}
}

func TestCheck_workflowsRowSkipsTheGoTestsTheScriptsRowRuns(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{
		".github/workflows/staging.yml": "name: staging\n",
		"scripts/a_test.go":             "package scripts\n\nfunc TestA(t *testing.T) {}\n",
	})
	code, stdout, stderr := h.check(t)
	joined := strings.Join(h.calls, "\n")
	if code != 0 || !strings.Contains(joined, "actionlint@v1.7.12") || strings.Contains(joined, "go -C scripts test") {
		t.Fatalf("workflow plus scripts change: %d %q %q\n%s", code, stdout, stderr, joined)
	}
}

func TestCheck_webRowRunsTheWebJobCommands(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.lookPath = func(string) (string, error) { return "/usr/bin/npm", nil }
	h.commit(t, map[string]string{"apps/web/src/a.ts": "export {}\n"})
	code, stdout, stderr := h.check(t)
	joined := strings.Join(h.calls, "\n")
	if code != 0 || !strings.Contains(stdout, "web             ok") ||
		!strings.Contains(joined, "npm ci --prefer-offline --no-audit --no-fund") ||
		!strings.Contains(joined, "npm test") || !strings.Contains(joined, "npm run build") {
		t.Fatalf("web change: %d %q %q\n%s", code, stdout, stderr, joined)
	}
}

func TestCheck_webRowWithoutNpmFailsNamingJustInstall(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/web/src/a.ts": "export {}\n"})
	h.check(t)
	if !strings.Contains(strings.Join(h.calls, "\n"), "just install") {
		t.Fatalf("failing row does not name just install\n%s", strings.Join(h.calls, "\n"))
	}
}
