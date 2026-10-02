package garden_test

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/tools/garden"
)

const plantedDeadcode = `[{"Name":"planted","Path":"example.com/m/internal/planted","Funcs":[
	{"Name":"planted","Position":{"File":"internal/planted/planted.go","Line":3,"Col":6},"Generated":false},
	{"Name":"generatedHelper","Position":{"File":"internal/planted/x.gen.go","Line":9,"Col":6},"Generated":true}]}]`

type fakeTools struct {
	results map[string][]garden.Result
	calls   []string
}

func (f *fakeTools) exec(_ context.Context, _, name string, args ...string) (garden.Result, error) {
	call := strings.Join(append([]string{filepath.Base(name)}, args...), " ")
	f.calls = append(f.calls, call)
	for prefix, queue := range f.results {
		if strings.HasPrefix(call, prefix) && len(queue) > 0 {
			f.results[prefix] = queue[1:]
			return queue[0], nil
		}
	}
	return garden.Result{}, nil
}

func cleanTree() map[string][]garden.Result {
	return map[string][]garden.Result{
		"git rev-parse": {{Stdout: []byte("/repo\napps/backend/\n")}},
		"golangci-lint": {{Stdout: []byte(`{"Issues":[]}`)}},
		"go run":        {{Stdout: []byte("[]")}},
	}
}

func moduleDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func run(t *testing.T, tools *fakeTools, mutants func(context.Context) ([]garden.Finding, error)) garden.Report {
	t.Helper()
	report, err := garden.Run(t.Context(), garden.Config{
		ModuleDir:    moduleDir(t),
		GolangciLint: "golangci-lint",
		Sqlc:         "/repo/.bin/sqlc",
		TempDir:      t.TempDir(),
		Exec:         tools.exec,
		Mutants:      mutants,
	})
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestRun_listsAPlantedUnusedFunctionUnderDeadCode(t *testing.T) {
	t.Parallel()
	results := cleanTree()
	results["go run"] = []garden.Result{{Stdout: []byte(plantedDeadcode)}}
	tools := &fakeTools{results: results}
	md := run(t, tools, nil).Markdown()
	want := "## Dead code (1)\n\n- `apps/backend/internal/planted/planted.go:3` unreachable func planted\n"
	if !strings.Contains(md, want) {
		t.Fatalf("report lacks the planted function under dead code:\n%s", md)
	}
	if !slices.Contains(tools.calls, "go run "+garden.Deadcode+" -test -json ./...") {
		t.Fatalf(
			"deadcode not run over the whole module with tests as roots; calls:\n%s",
			strings.Join(tools.calls, "\n"),
		)
	}
}

func TestRun_groupsLintByLinterAndDriftByFileWithTheFirstChangedLine(t *testing.T) {
	t.Parallel()
	results := cleanTree()
	results["golangci-lint"] = []garden.Result{{Code: 1, Stdout: []byte(`{"Issues":[
		{"FromLinter":"funlen","Text":"Function 'a' is too long (61 > 60)","Pos":{"Filename":"internal/a/a.go","Line":7}},
		{"FromLinter":"goconst","Text":"string ` + "`x`" + ` has 3 occurrences","Pos":{"Filename":"internal/b/b.go","Line":2}}]}`)}}
	results["git status"] = []garden.Result{
		{},
		{Stdout: []byte(" M apps/backend/internal/x/x.gen.go\n?? docs/reference/new.md\n")},
	}
	results["git diff"] = []garden.Result{
		{Stdout: []byte("diff --git a/apps/backend/internal/x/x.gen.go b/apps/backend/internal/x/x.gen.go\n" +
			"--- a/apps/backend/internal/x/x.gen.go\n+++ b/apps/backend/internal/x/x.gen.go\n@@ -12 +12,2 @@ func x\n-a\n+b\n@@ -40 +41 @@\n")},
	}
	tools := &fakeTools{results: results}
	md := run(t, tools, nil).Markdown()
	for _, want := range []string{
		"| Dead code | 0 |\n| Candidate lint | 2 |\n| Surviving mutants | skipped |\n| Generator drift | 2 |\n",
		"### funlen (1)\n\n- `apps/backend/internal/a/a.go:7` Function 'a' is too long (61 > 60)\n\n### goconst (1)\n",
		"- `apps/backend/internal/x/x.gen.go:12` the committed file differs from what the generators write\n",
		"- `docs/reference/new.md:1` a generator writes this file and it is not committed\n",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("report lacks %q:\n%s", want, md)
		}
	}
	generators := []string{"go generate ./...", "sqlc generate", "bash /repo/scripts/gen-docs.sh"}
	for _, g := range generators {
		if !slices.Contains(tools.calls, g) {
			t.Fatalf("generator %q not run; calls:\n%s", g, strings.Join(tools.calls, "\n"))
		}
	}
}

func TestRun_recordsAFailedCheckAndStillRunsTheRest(t *testing.T) {
	t.Parallel()
	results := cleanTree()
	results["git status"] = []garden.Result{{Stdout: []byte(" M apps/backend/go.mod\n")}}
	results["go run"] = []garden.Result{{Code: 1, Stderr: "go: cannot find module"}}
	tools := &fakeTools{results: results}
	mutants := func(context.Context) ([]garden.Finding, error) {
		return []garden.Finding{
			{Rule: "CONDITIONALS_NEGATION", File: "internal/a/a.go", Line: 4, Text: "survived"},
		}, nil
	}
	report := run(t, tools, mutants)
	failed := report.Failed()
	if len(failed) != 2 || failed[0].Kind != garden.KindDeadCode || failed[1].Kind != garden.KindDrift {
		t.Fatalf("failed sections %+v, want dead code and drift", failed)
	}
	md := report.Markdown()
	if !strings.Contains(md, "- `apps/backend/internal/a/a.go:4` survived\n") {
		t.Fatalf("mutant not listed with a repo path:\n%s", md)
	}
	if slices.Contains(tools.calls, "go generate ./...") {
		t.Fatal("generators ran on a dirty tree")
	}
}

func TestRun_failsWhenTheToolCannotStart(t *testing.T) {
	t.Parallel()
	exec := func(context.Context, string, string, ...string) (garden.Result, error) {
		return garden.Result{}, errs.New(errs.CodeInternal, "test.exec")
	}
	_, err := garden.Run(t.Context(), garden.Config{ModuleDir: moduleDir(t), Exec: exec})
	if err == nil || errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err %v, want the exec failure", err)
	}
}

func TestCandidateConfig_tightensTheEnforcedConfig(t *testing.T) {
	t.Parallel()
	out, err := garden.CandidateConfig(moduleDir(t))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Linters struct {
			Enable   []string `yaml:"enable"`
			Settings struct {
				Cyclop map[string]int `yaml:"cyclop"`
				Funlen map[string]int `yaml:"funlen"`
			} `yaml:"settings"`
			Exclusions struct {
				Rules []any `yaml:"rules"`
			} `yaml:"exclusions"`
		} `yaml:"linters"`
	}
	if err := yaml.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	l := cfg.Linters
	if l.Settings.Cyclop["max-complexity"] != 10 || l.Settings.Funlen["lines"] != 60 ||
		l.Settings.Funlen["statements"] != 40 {
		t.Fatalf("settings %+v, want cyclop 10, funlen 60 lines and the enforced 40 statements", l.Settings)
	}
	if !slices.Contains(l.Enable, "errcheck") || !slices.Contains(l.Enable, "dupl") || len(l.Exclusions.Rules) == 0 {
		t.Fatalf("enable %v with %d exclusion rules, want the enforced linters, the candidates and the exclusions",
			l.Enable, len(l.Exclusions.Rules))
	}
	if len(slices.Compact(slices.Sorted(slices.Values(l.Enable)))) != len(l.Enable) {
		t.Fatalf("a linter is enabled twice: %v", l.Enable)
	}
}

func TestRun_recordsTheCauseWhenACheckCannotRun(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		results map[string][]garden.Result
		kind    garden.Kind
		cause   string
	}{
		{
			"deadcode prints garbage",
			map[string][]garden.Result{"go run": {{Stdout: []byte("not json")}}},
			garden.KindDeadCode, "garden.deadcode: decode_failed",
		},
		{
			"golangci-lint crashes",
			map[string][]garden.Result{"golangci-lint": {{Code: 3, Stderr: "panic: boom\n"}}},
			garden.KindLint, "exited 3: panic: boom",
		},
		{
			"golangci-lint prints garbage",
			map[string][]garden.Result{"golangci-lint": {{Code: 1, Stdout: []byte("not json")}}},
			garden.KindLint, "garden.lint: decode_failed",
		},
		{
			"git status fails",
			map[string][]garden.Result{"git status": {{Code: 128, Stderr: "not a git repository"}}},
			garden.KindDrift, "git status --porcelain --untracked-files=all exited 128: not a git repository",
		},
		{
			"a generator fails",
			map[string][]garden.Result{"go generate": {{Code: 1, Stderr: "gen: boom"}}},
			garden.KindDrift, "go generate ./... exited 1: gen: boom",
		},
		{
			"git status fails after the generators",
			map[string][]garden.Result{"git status": {{}, {Code: 128, Stderr: "index.lock exists"}}},
			garden.KindDrift, "exited 128: index.lock exists",
		},
		{
			"git diff fails",
			map[string][]garden.Result{"git diff": {{Code: 129, Stderr: "bad flag"}}},
			garden.KindDrift, "git diff -U0 --no-color --no-ext-diff exited 129: bad flag",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			results := cleanTree()
			maps.Copy(results, tc.results)
			failed := run(t, &fakeTools{results: results}, nil).Failed()
			if len(failed) != 1 || failed[0].Kind != tc.kind || !strings.Contains(failed[0].Err.Error(), tc.cause) {
				t.Fatalf("failed sections %+v, want only %s failing with %q", failed, tc.kind, tc.cause)
			}
		})
	}
}

func TestRun_failsTheLintCheckWhenItCannotBuildOrWriteTheCandidateConfig(t *testing.T) {
	t.Parallel()
	for _, cfg := range []garden.Config{
		{ModuleDir: t.TempDir(), TempDir: t.TempDir()},
		{ModuleDir: moduleDir(t), TempDir: filepath.Join(t.TempDir(), "missing")},
	} {
		cfg.Exec = (&fakeTools{results: cleanTree()}).exec
		report, err := garden.Run(t.Context(), cfg)
		failed := report.Failed()
		if err != nil || len(failed) != 1 || failed[0].Kind != garden.KindLint ||
			errs.CodeOf(failed[0].Err) != errs.CodeInternal {
			t.Fatalf("module %s, temp %s: failed %+v, err %v; want only the lint check failing", cfg.ModuleDir,
				cfg.TempDir, failed, err)
		}
	}
}

func TestRun_failsWhenGitDoesNotPrintTheRootAndThePrefix(t *testing.T) {
	t.Parallel()
	tools := &fakeTools{results: map[string][]garden.Result{"git rev-parse": {{Stdout: []byte("/repo")}}}}
	if _, err := garden.Run(t.Context(), garden.Config{ModuleDir: moduleDir(t), Exec: tools.exec}); errs.CodeOf(
		err,
	) != errs.CodeDecodeFailed {
		t.Fatalf("err %v, want decode_failed", err)
	}
}

func TestRun_listsAFileTheGeneratorsDelete(t *testing.T) {
	t.Parallel()
	results := cleanTree()
	results["git status"] = []garden.Result{{}, {Stdout: []byte(" D apps/backend/internal/x/x.gen.go\n")}}
	md := run(t, &fakeTools{results: results}, nil).Markdown()
	want := "- `apps/backend/internal/x/x.gen.go:1` a generator deletes this committed file\n"
	if !strings.Contains(md, want) {
		t.Fatalf("report lacks %q:\n%s", want, md)
	}
}

func TestRun_makesAnAbsolutePathRepoRelativeAndKeepsOneItCannot(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ top, file, want string }{
		{"/repo", "/repo/apps/backend/internal/a/a.go", "apps/backend/internal/a/a.go"},
		{"repo", "/elsewhere/a.go", "/elsewhere/a.go"},
	} {
		results := cleanTree()
		results["git rev-parse"] = []garden.Result{{Stdout: []byte(tc.top + "\napps/backend/\n")}}
		results["golangci-lint"] = []garden.Result{{Code: 1, Stdout: []byte(
			`{"Issues":[{"FromLinter":"funlen","Text":"long","Pos":{"Filename":"` + tc.file + `","Line":7}}]}`,
		)}}
		md := run(t, &fakeTools{results: results}, nil).Markdown()
		if want := "- `" + tc.want + ":7` long\n"; !strings.Contains(md, want) {
			t.Fatalf("top %s: report lacks %q:\n%s", tc.top, want, md)
		}
	}
}

func TestCandidateConfig_failsOnAMissingDirAMissingConfigAndBadYAML(t *testing.T) {
	t.Parallel()
	bad := t.TempDir()
	if err := os.WriteFile(filepath.Join(bad, ".golangci.yml"), []byte("linters: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		dir  string
		code errs.Code
	}{
		{filepath.Join(t.TempDir(), "missing"), errs.CodeInternal},
		{t.TempDir(), errs.CodeInternal},
		{bad, errs.CodeDecodeFailed},
	} {
		if _, err := garden.CandidateConfig(tc.dir); errs.CodeOf(err) != tc.code {
			t.Fatalf("CandidateConfig(%s) err %v, want %s", tc.dir, err, tc.code)
		}
	}
}

func TestRun_acceptsGitPrintingTheRootAndThePrefixWithoutATrailingNewline(t *testing.T) {
	t.Parallel()
	results := cleanTree()
	results["git rev-parse"] = []garden.Result{{Stdout: []byte("/repo\napps/backend/")}}
	results["go run"] = []garden.Result{{Stdout: []byte(plantedDeadcode)}}
	md := run(t, &fakeTools{results: results}, nil).Markdown()
	want := "- `apps/backend/internal/planted/planted.go:3` unreachable func planted\n"
	if !strings.Contains(md, want) {
		t.Fatalf("report lacks %q:\n%s", want, md)
	}
}

func TestRun_countsOnlyPorcelainLinesThatNameAFile(t *testing.T) {
	t.Parallel()
	results := cleanTree()
	results["git status"] = []garden.Result{{}, {Stdout: []byte("?? \n M apps/backend/internal/x/x.gen.go\n")}}
	md := run(t, &fakeTools{results: results}, nil).Markdown()
	for _, want := range []string{
		"| Generator drift | 1 |\n",
		"- `apps/backend/internal/x/x.gen.go:1` the committed file differs from what the generators write\n",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("report lacks %q:\n%s", want, md)
		}
	}
}

func TestRun_skipsAHunkHeaderWithoutTheNewRangeAndTakesTheNextOne(t *testing.T) {
	t.Parallel()
	results := cleanTree()
	results["git status"] = []garden.Result{{}, {Stdout: []byte(" M apps/backend/internal/x/x.gen.go\n")}}
	results["git diff"] = []garden.Result{{Stdout: []byte(
		"+++ b/apps/backend/internal/x/x.gen.go\n@@ -1\n@@ -12 +12,2 @@\n",
	)}}
	md := run(t, &fakeTools{results: results}, nil).Markdown()
	want := "- `apps/backend/internal/x/x.gen.go:12` the committed file differs from what the generators write\n"
	if !strings.Contains(md, want) {
		t.Fatalf("report lacks %q:\n%s", want, md)
	}
}
