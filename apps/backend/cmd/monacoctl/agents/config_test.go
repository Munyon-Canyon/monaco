package agents

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

type variableGet struct {
	out   string
	err   error
	calls int
}

func (v *variableGet) run(_ context.Context, _, _, name string, args ...string) ([]byte, error) {
	v.calls++
	if got := name + " " + strings.Join(args, " "); got != "gh variable get FEATURE_BRANCH --repo o/r" {
		return nil, errors.New("unexpected " + got)
	}
	return []byte(v.out), v.err
}

func autoConfig() Config { return Config{Repo: "o/r", FeatureBranch: "auto"} }

func TestResolveFeatureBranch_keepsAnExplicitNameWithoutAsking(t *testing.T) {
	t.Parallel()
	v := &variableGet{out: "domain-core-2\n"}
	got, err := resolveFeatureBranch(t.Context(), v.run, []string{"MONACO_FEATURE_BRANCH=ledger-9"}, "",
		Config{Repo: "o/r", FeatureBranch: "domain-core-12"})
	if err != nil || got != "domain-core-12" || v.calls != 0 {
		t.Fatalf("got %q %v after %d gh calls, want the configured name and no lookup", got, err, v.calls)
	}
}

func TestResolveFeatureBranch_autoPrefersTheEnvOverTheRepoVariable(t *testing.T) {
	t.Parallel()
	v := &variableGet{out: "domain-core-2\n"}
	environ := []string{"MONACO_FEATURE_BRANCH=domain-core-12"}
	got, err := resolveFeatureBranch(t.Context(), v.run, environ, "", autoConfig())
	if err != nil || got != "domain-core-12" || v.calls != 0 {
		t.Fatalf("got %q %v after %d gh calls, want the env and no gh call", got, err, v.calls)
	}
}

func TestResolveFeatureBranch_autoFallsBackToTheRepoVariable(t *testing.T) {
	t.Parallel()
	for _, environ := range [][]string{nil, {"MONACO_FEATURE_BRANCH="}} {
		v := &variableGet{out: "domain-core-12\n"}
		got, err := resolveFeatureBranch(t.Context(), v.run, environ, "", autoConfig())
		if err != nil || got != "domain-core-12" || v.calls != 1 {
			t.Errorf("%v: got %q %v after %d gh calls", environ, got, err, v.calls)
		}
	}
}

func TestResolveFeatureBranch_autoFailsNamingBothSources(t *testing.T) {
	t.Parallel()
	for v, reason := range map[*variableGet]string{
		{err: errors.New("HTTP 404")}: "HTTP 404",
		{out: " \n"}:                  "printed nothing",
	} {
		_, err := resolveFeatureBranch(t.Context(), v.run, nil, "", autoConfig())
		want := configPath + `: feature_branch = "auto", but MONACO_FEATURE_BRANCH is unset and ` +
			"gh variable get FEATURE_BRANCH --repo o/r " + reason
		if err == nil || !strings.Contains(cliText(err), want) {
			t.Errorf("%s: %v, want %q", reason, err, want)
		}
	}
}

func TestLoad_resolvesAnAutoFeatureBranchForEveryCommand(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		env, variable, want string
	}{
		{"MONACO_FEATURE_BRANCH=domain-core-12", "", "domain-core-12"},
		{"", "backend-rewrite-4\n", "backend-rewrite-4"},
		{"", "", ""},
	} {
		f := newFixture(t)
		writeFile(t, filepath.Join(f.dir, configPath), strings.Replace(testConfig, `"fb"`, `"auto"`, 1))
		f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
			if name == "gh" && len(args) > 1 && args[0] == "variable" {
				return []byte(tc.variable), nil
			}
			return hostless(ctx, dir, stdin, name, args...)
		}
		environ := f.env
		if tc.env != "" {
			environ = append(environ, tc.env)
		}
		env, err := load(t.Context(), environ, f.dir, f.cached(f.run))
		switch {
		case tc.want == "" && (err == nil || !strings.Contains(cliText(err), "MONACO_FEATURE_BRANCH is unset")):
			t.Errorf("no branch: %v", err)
		case tc.want != "" && (err != nil || env.Config.FeatureBranch != tc.want):
			t.Errorf("%+v: got %v %v, want %s", tc, env, err, tc.want)
		}
	}
}
