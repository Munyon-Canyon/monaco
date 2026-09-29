package verify

import (
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
	tools "github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func TestParseArgs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		args []string
		want Target
		err  string
	}{
		{nil, Target{}, ""},
		{[]string{"all"}, Target{}, ""},
		{[]string{"flow", "00", "--outcome", "InvalidInput"}, Target{Flow: "00", Outcome: "InvalidInput"}, ""},
		{[]string{"all", "--crash-at", "after-publish"}, Target{CrashAt: "after-publish"}, ""},
		{[]string{"flow"}, Target{}, "bad arguments: flow needs a value"},
		{[]string{"--crash-at", "after-lunch"}, Target{}, "bad arguments: --crash-at after-lunch is not a faultpoint"},
		{[]string{"everything"}, Target{}, `bad arguments: unknown argument "everything"`},
	} {
		got, err := ParseArgs(tc.args)
		if tc.err != "" {
			if err == nil || err.Error() != tc.err {
				t.Errorf("ParseArgs(%q) = %v, want %q", tc.args, err, tc.err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("ParseArgs(%q) = %+v, %v, want %+v", tc.args, got, err, tc.want)
		}
	}
}

func fakeGo(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return writeScript(t, "go", `echo "$@" > "$(dirname "$0")/args"
while [ $# -gt 0 ]; do [ "$1" = "-o" ] && out="$2"; shift; done
for b in api worker fakes; do ln -s "`+exe+`" "$out$b"; done
`)
}

const plantedFlows = tools.Header + "\n" +
	"90\tHealth\tsystem\tGET /healthz\tHealth\t\t\tok;crash:after-publish\tbuilt\tdocs/x.md#health\n" +
	"91\tLater\tsystem\tGET /later\tLater\t\t\tok\tplanned\tdocs/x.md#later\n"

func plantedScripts() map[string]flows.Script {
	health := func(s *scenario.Scenario) {
		s.When(scenario.Anonymous(), scenario.Get("/healthz"), scenario.ExpectStatus(http.StatusOK))
	}
	return map[string]flows.Script{
		"F90HealthOK": health,
		"F90HealthCrashAfterPublish": func(s *scenario.Scenario) {
			health(s)
			s.Then(scenario.PublishCrashingAt(faultpoint.AfterPublish))
		},
	}
}

func testConfig(t *testing.T, mode string) (Config, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	o := testOptions(t, mode)
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(o.Dir, "migrations"), filepath.Join(dir, "migrations")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, tools.File), []byte(plantedFlows), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	return Config{
		Dir: dir, Environ: o.Environ, Go: fakeGo(t), Atlas: o.Atlas, Budget: o.Budget, Postgres: o.Postgres,
		Modules: func(module.Deps) module.Set { return nil }, Scripts: plantedScripts(),
		Stdout: &stdout, Stderr: &stderr,
	}, &stdout, &stderr
}

func TestRun_buildsTheBinariesRunsEveryOutcomeAndTearsTheStackDown(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		target Target
		tags   string
		want   string
	}{
		{"crash", Target{CrashAt: "after-publish"}, "build -cover -tags faultpoints -o ", "PASS flow 90 crash:after-publish"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, stdout, stderr := testConfig(t, "ok")
			if code := Run(t.Context(), cfg, tc.target); code != 0 {
				t.Fatalf("Run = %d\n%s\n%s", code, stdout, stderr)
			}
			if !strings.Contains(stdout.String(), "healthy: api http://127.0.0.1:") ||
				!strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("stdout = %q, want %q", stdout, tc.want)
			}
			args, err := os.ReadFile(filepath.Join(filepath.Dir(cfg.Go), "args"))
			if err != nil || !strings.HasPrefix(string(args), tc.tags) ||
				!strings.Contains(string(args), "./cmd/api ./cmd/worker ./cmd/fakes") {
				t.Fatalf("go args = %q, %v", args, err)
			}
			if _, err := os.Stat(filepath.Join(cfg.Dir, ".verify", "cover")); err != nil {
				t.Fatalf("coverage dir: %v", err)
			}
		})
	}
}

func TestRun_failsWithTheReasonAndExitOne(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		edit func(t *testing.T, cfg *Config)
		want string
	}{
		{"tempdir", func(_ *testing.T, cfg *Config) { cfg.TempDir = "/nonexistent" }, "binaries dir"},
		{
			"build", func(t *testing.T, cfg *Config) {
				t.Helper()
				cfg.Go = writeScript(t, "go", "echo broken; exit 2")
			},
			"go build: exit status 2\nbroken",
		},
		{"image", func(t *testing.T, cfg *Config) {
			t.Helper()
			cfg.Postgres = nil
			cfg.Docker, _ = fakeDocker(t, "exit 0", "exit 0", "exit 1")
		}, "pull failed"},
		{"cover", func(t *testing.T, cfg *Config) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(cfg.Dir, ".verify"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}, "coverage dir"},
		{"stack", func(_ *testing.T, cfg *Config) { cfg.Atlas = "/nonexistent/atlas" }, "migrate apply"},
		{"flows", func(t *testing.T, cfg *Config) {
			t.Helper()
			if err := os.Remove(filepath.Join(cfg.Dir, tools.File)); err != nil {
				t.Fatal(err)
			}
		}, "read flows.tsv"},
		{"script", func(_ *testing.T, cfg *Config) { cfg.Scripts = nil }, "flow 90 outcome ok has no script F90HealthOK"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, _, stderr := testConfig(t, "ok")
			tc.edit(t, &cfg)
			if code := Run(t.Context(), cfg, Target{}); code != 1 || !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("Run = %d, stderr %q, want 1 and %q", code, stderr, tc.want)
			}
		})
	}
}

func gitInit(t *testing.T, dir string) string {
	t.Helper()
	git := func(args ...string) string {
		base := []string{
			"-C", dir, "-c", "user.name=verify", "-c", "user.email=verify@example.com", "-c", "commit.gpgsign=false",
		}
		out, err := exec.CommandContext(t.Context(), "git", append(base, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("/.verify/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("init", "-q")
	git("add", "-A")
	git("commit", "-q", "-m", "flows")
	return git("rev-parse", "HEAD")
}

func TestRun_stampsEvidenceWithTheCommitAndDirtyFlagEvenOverBudget(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		edit   func(t *testing.T, cfg *Config)
		code   int
		dirty  bool
		result string
	}{
		{"clean", func(*testing.T, *Config) {}, 0, false, resultPass},
		{"dirty", func(t *testing.T, cfg *Config) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(cfg.Dir, "wip"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}, 0, true, resultPass},
		{"over budget", func(_ *testing.T, cfg *Config) { cfg.Budget.Total = time.Nanosecond }, 1, false, resultOverBudget},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, stdout, stderr := testConfig(t, "ok")
			head := gitInit(t, cfg.Dir)
			tc.edit(t, &cfg)
			if code := Run(t.Context(), cfg, Target{Flow: "90"}); code != tc.code {
				t.Fatalf("Run = %d, want %d\n%s\n%s", code, tc.code, stdout, stderr)
			}
			ev := readEvidence(t, filepath.Join(cfg.Dir, ".verify", "90.json"))
			if ev.Commit != head || ev.Dirty != tc.dirty || ev.Result != tc.result {
				t.Fatalf("evidence commit %q dirty %v result %q, want %q %v %q",
					ev.Commit, ev.Dirty, ev.Result, head, tc.dirty, tc.result)
			}
		})
	}
}

func TestBuild_withoutACrashPointBuildsWithoutTheFaultpointsTag(t *testing.T) {
	t.Parallel()
	goBin := fakeGo(t)
	if _, err := build(t.Context(), goBin, t.TempDir(), t.TempDir(), false); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(filepath.Join(filepath.Dir(goBin), "args"))
	if err != nil || !strings.HasPrefix(string(args), "build -cover -o ") {
		t.Fatalf("go args = %q, %v", args, err)
	}
}
