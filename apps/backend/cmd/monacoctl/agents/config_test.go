package agents

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
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

func repoGH(t *testing.T, status int, body string) *GitHub {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &GitHub{
		API: srv.URL, Repo: "o/r", HTTP: srv.Client(),
		Token: func(context.Context) (string, error) { return "tok", nil },
	}
}

func TestResolveFeatureBranch_keepsAnExplicitNameWithoutAsking(t *testing.T) {
	t.Parallel()
	v := &variableGet{out: "domain-core-2\n"}
	got, note, err := resolveFeatureBranch(t.Context(), v.run, nil, []string{"MONACO_FEATURE_BRANCH=ledger-9"}, "",
		Config{Repo: "o/r", FeatureBranch: "domain-core-12"})
	if err != nil || got != "domain-core-12" || note != "" || v.calls != 0 {
		t.Fatalf("got %q note %q %v after %d gh calls, want the configured name and no lookup", got, note, err, v.calls)
	}
}

func TestResolveFeatureBranch_autoPrefersTheEnvOverTheRepoVariable(t *testing.T) {
	t.Parallel()
	v := &variableGet{out: "domain-core-2\n"}
	environ := []string{"MONACO_FEATURE_BRANCH=domain-core-12"}
	got, note, err := resolveFeatureBranch(t.Context(), v.run, nil, environ, "", autoConfig())
	if err != nil || got != "domain-core-12" || note != "" || v.calls != 0 {
		t.Fatalf("got %q note %q %v after %d gh calls, want the env and no gh call", got, note, err, v.calls)
	}
}

func TestResolveFeatureBranch_autoFallsBackToTheRepoVariable(t *testing.T) {
	t.Parallel()
	for _, environ := range [][]string{nil, {"MONACO_FEATURE_BRANCH="}} {
		v := &variableGet{out: "domain-core-12\n"}
		got, note, err := resolveFeatureBranch(t.Context(), v.run, nil, environ, "", autoConfig())
		if err != nil || got != "domain-core-12" || note != "" || v.calls != 1 {
			t.Errorf("%v: got %q note %q %v after %d gh calls", environ, got, note, err, v.calls)
		}
	}
}

func TestResolveFeatureBranch_autoReadsTheRepoDefaultWhenTheVariableFails(t *testing.T) {
	t.Parallel()
	for _, v := range []*variableGet{{err: errors.New("HTTP 403")}, {out: " \n"}} {
		gh := repoGH(t, http.StatusOK, `{"default_branch":"staging"}`)
		got, note, err := resolveFeatureBranch(t.Context(), v.run, gh, nil, "", autoConfig())
		wantNote := "feature branch staging (repo default branch; gh variable get failed:"
		if err != nil || got != "staging" || !strings.Contains(note, wantNote) {
			t.Fatalf("got %q note %q err %v", got, note, err)
		}
	}
}

func TestResolveFeatureBranch_autoFailsNamingEverySource(t *testing.T) {
	t.Parallel()
	cases := []struct {
		v      *variableGet
		status int
		body   string
		reason string
		repo   string
	}{
		{&variableGet{err: errors.New("HTTP 404")}, http.StatusForbidden, `{"message":"no"}`, "HTTP 404", "403"},
		{&variableGet{out: " \n"}, http.StatusOK, `{}`, "printed nothing", "printed nothing"},
	}
	for _, tc := range cases {
		gh := repoGH(t, tc.status, tc.body)
		_, _, err := resolveFeatureBranch(t.Context(), tc.v.run, gh, nil, "", autoConfig())
		want := configPath + `: feature_branch = "auto", but MONACO_FEATURE_BRANCH is unset, ` +
			"gh variable get FEATURE_BRANCH --repo o/r " + tc.reason + ", and GET /repos/o/r default_branch"
		if err == nil || !strings.Contains(cliText(err), want) || !strings.Contains(cliText(err), tc.repo) {
			t.Errorf("%s: %v, want %q and %q", tc.reason, err, want, tc.repo)
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
		case tc.want == "" && (err == nil || !strings.Contains(cliText(err), "MONACO_FEATURE_BRANCH is unset") ||
			!strings.Contains(cliText(err), "default_branch")):
			t.Errorf("no branch: %v", err)
		case tc.want != "" && (err != nil || env.Config.FeatureBranch != tc.want):
			t.Errorf("%+v: got %v %v, want %s", tc, env, err, tc.want)
		}
	}
}

func TestVerifyPlan_printsTheRepoDefaultWhenTheVariableAPIFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	writeFile(t, filepath.Join(f.dir, configPath), strings.Replace(testConfig, `"fb"`, `"auto"`, 1))
	f.hub.on("GET /repos/o/r", `{"default_branch":"staging"}`)
	f.hub.on(get("/pulls/5"), pr(5, "h", "fb", "Part of #40"))
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 1}})
	f.owner(t, Record{Ticket: 40, Model: sonnet, State: Running})
	prev := f.run
	f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "gh" && len(args) > 0 && args[0] == "variable" {
			return nil, errors.New(
				"gh: Access to this GitHub Actions path is not permitted (HTTP 403)",
			)
		}
		return prev(ctx, dir, stdin, name, args...)
	}
	code, stdout, stderr := f.agents(t, "verify-plan", "5")
	note := "feature branch staging (repo default branch; gh variable get failed: " +
		"gh: Access to this GitHub Actions path is not permitted (HTTP 403))"
	f.hub.mu.Lock()
	_, hit := f.hub.sent["GET /repos/o/r"]
	f.hub.mu.Unlock()
	if code != 0 || !strings.Contains(stderr, note) || !strings.Contains(stdout, "verifier ") || !hit {
		t.Fatalf("code=%d hit=%v stdout=%q stderr=%q", code, hit, stdout, stderr)
	}
}

func TestParseConfig_readsTheBatchTable(t *testing.T) {
	t.Parallel()
	c, err := parseConfig(strings.NewReader(testConfig + "shared = [\n  # regenerated\n  \"a/**\",\n  \"b.go\",\n]\n"))
	if err != nil || c.Batch != 2 || !slices.Equal(c.Shared, []string{"a/**", "b.go"}) {
		t.Fatalf("batch: %d %q %v", c.Batch, c.Shared, err)
	}
	c, err = parseConfig(strings.NewReader(testConfig + "shared = [\"a/**\", \"b.go\"]\n"))
	if err != nil || !slices.Equal(c.Shared, []string{"a/**", "b.go"}) {
		t.Fatalf("one line: %q %v", c.Shared, err)
	}
	for body, want := range map[string]string{
		"shared = \"a/**\"\n":         `:10: want a list of quoted strings such as ["a/**"]`,
		"shared = [a/**]\n":           `:10: want a list of quoted strings such as ["a/**"], got a/**`,
		"shared = [\n  \"a/**\",\n":   `:10: want a list of quoted strings`,
		"size = 2\nshared = [\n\"a\"": `:11: want a list of quoted strings`,
	} {
		if _, err := parseConfig(strings.NewReader(testConfig + body)); err == nil ||
			!strings.Contains(cliText(err), configPath+want) {
			t.Errorf("%q: %v", body, cliText(err))
		}
	}
	if c, err := parseConfig(strings.NewReader(testConfig + "other = 1\n")); err != nil ||
		!slices.Equal(c.Unknown, []string{"batch.other"}) {
		t.Errorf("unknown batch key: %q %v", c.Unknown, err)
	}
	missing := strings.Replace(testConfig, "size = 2\n", "", 1)
	if _, err := parseConfig(strings.NewReader(missing)); err == nil ||
		!strings.Contains(cliText(err), "missing batch.size") {
		t.Fatalf("missing size: %v", err)
	}
}

func TestParseConfig_readsTheCommittedConfig(t *testing.T) {
	t.Parallel()
	c := committedConfig(t)
	if c.Batch < 4 || len(c.Shared) == 0 {
		t.Fatalf("committed config: batch %d, shared %q", c.Batch, c.Shared)
	}
}

func committedConfig(t *testing.T) Config {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "..", "..", "..", configPath))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	c, err := parseConfig(f)
	if err != nil || len(c.Unknown) > 0 {
		t.Fatalf("%s: unknown keys %q, %v", configPath, c.Unknown, err)
	}
	return c
}

func TestParseConfig_readsTheCheckSlots(t *testing.T) {
	t.Parallel()
	if c, err := parseConfig(strings.NewReader(testConfig)); err != nil || c.Slots != 2 {
		t.Fatalf("default slots: %d %v", c.Slots, err)
	}
	if c, err := parseConfig(strings.NewReader(testConfig + "\n[check]\nslots = 3\n")); err != nil || c.Slots != 3 {
		t.Fatalf("slots: %d %v", c.Slots, err)
	}
	if c := committedConfig(t); c.Slots != 2 {
		t.Fatalf("committed slots: %d", c.Slots)
	}
	for body, want := range map[string]string{
		"[check]\nslots = 0\n":     "check.slots: want at least 1, got 0",
		"[check]\nslots = \"2\"\n": "int:",
	} {
		if _, err := parseConfig(strings.NewReader(testConfig + "\n" + body)); err == nil ||
			!strings.Contains(cliText(err), want) {
			t.Errorf("%q: %v", body, cliText(err))
		}
	}
	if c, err := parseConfig(strings.NewReader(testConfig + "\n[check]\nother = 1\n")); err != nil ||
		!slices.Equal(c.Unknown, []string{"check.other"}) {
		t.Errorf("unknown check key: %q %v", c.Unknown, err)
	}
}

func TestParseConfig_readsTheDispatchLoadCeilingAndRejectsBadValues(t *testing.T) {
	t.Parallel()
	base := strings.TrimSuffix(testConfig, "[batch]\nsize = 2\n") + "[batch]\nsize = 2\n"
	cfg, err := parseConfig(strings.NewReader(base))
	if err != nil || cfg.MaxLoad != 12 {
		t.Fatalf("default: %v %v", cfg.MaxLoad, err)
	}
	cfg, err = parseConfig(strings.NewReader(base + "[dispatch]\nmax_load = 8\n"))
	if err != nil || cfg.MaxLoad != 8 {
		t.Fatalf("set: %v %v", cfg.MaxLoad, err)
	}
	for tail, want := range map[string]string{
		"[dispatch]\nmax_load = 0\n":    "dispatch.max_load: want above 0, got 0",
		"[dispatch]\nmax_load = high\n": "int: strconv.Atoi",
		"[dispatch]\nmax_load\n":        "want key = value",
	} {
		if _, err := parseConfig(strings.NewReader(base + tail)); err == nil || !strings.Contains(cliText(err), want) {
			t.Errorf("%q: got %v, want %q", tail, err, want)
		}
	}
	cfg, err = parseConfig(strings.NewReader(base + "[dispatch]\nlanes_hint = 3\n"))
	if err != nil || cfg.MaxLoad != 12 || !slices.Equal(cfg.Unknown, []string{"dispatch.lanes_hint"}) {
		t.Errorf("unknown dispatch key: %q %v", cfg.Unknown, err)
	}
}

func TestLoad_appliesTheLocalCapacityFromEveryWorktree(t *testing.T) {
	t.Parallel()
	f := newFixtureFrom(t, rootedRepo)
	wt := filepath.Join(t.TempDir(), "wt")
	git(t, f.dir, "worktree", "add", "-q", wt, "fb")
	writeFile(t, filepath.Join(wt, configPath), testConfig)
	tracked, err := parseConfig(strings.NewReader(testConfig))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{f.dir, wt} {
		env, err := load(t.Context(), f.env, dir, hostless)
		if err != nil || !reflect.DeepEqual(env.Config, tracked) || env.localConfig != "" {
			t.Fatalf("%s without a local file: %+v %v", dir, env, err)
		}
	}
	writeFile(t, filepath.Join(f.dir, ".git", localConfigPath),
		"lanes = 6\ntracking = 512\nmilestone = \"m12\"\n[check]\nslots = 4\n")
	want := tracked
	want.Lanes, want.Slots, want.Tracking, want.Milestone = 6, 4, 512, "m12"
	for _, dir := range []string{f.dir, wt} {
		env, err := load(t.Context(), f.env, dir, hostless)
		if err != nil || !reflect.DeepEqual(env.Config, want) ||
			!strings.HasSuffix(env.localConfig, filepath.Join(".git", localConfigPath)) {
			t.Fatalf("%s with a local file: %+v %v", dir, env, err)
		}
	}
	writeFile(t, filepath.Join(f.dir, ".git", localConfigPath), "repo = \"x/y\"\n")
	_, err = load(t.Context(), f.env, wt, hostless)
	if err == nil || !strings.Contains(cliText(err), `.monaco/agents.local.toml:1: unknown key "repo"`) {
		t.Fatalf("a policy key in the local file: %v", cliText(err))
	}
	monaco := filepath.Join(f.dir, ".git", ".monaco")
	if err := os.RemoveAll(monaco); err != nil {
		t.Fatal(err)
	}
	writeFile(t, monaco, "not a directory\n")
	_, err = load(t.Context(), f.env, wt, hostless)
	if err == nil || !strings.Contains(err.Error(), "read local config") {
		t.Fatalf("an unreadable local file: %v", err)
	}
}

func TestApplyLocalConfig_acceptsCapacityTrackingAndMilestoneWithinBounds(t *testing.T) {
	t.Parallel()
	tracked, err := parseConfig(strings.NewReader(testConfig))
	if err != nil {
		t.Fatal(err)
	}
	got, err := applyLocalConfig(tracked, strings.NewReader("[dispatch]\nmax_load = 20\n"))
	if err != nil || got.MaxLoad != 20 || got.Lanes != tracked.Lanes || got.Slots != tracked.Slots {
		t.Fatalf("max_load: %+v %v", got, err)
	}
	applied := tracked
	applied.Tracking, applied.Milestone = 512, "m12"
	got, err = applyLocalConfig(tracked, strings.NewReader("tracking = 512\nmilestone = \"m12\"\n"))
	if err != nil || !reflect.DeepEqual(got, applied) {
		t.Fatalf("tracking and milestone: %+v %v, want %+v", got, err, applied)
	}
	for body, want := range map[string]string{
		"repo = \"x/y\"\n":               `.monaco/agents.local.toml:1: unknown key "repo"`,
		"lanes = 6\n[batch]\nsize = 3\n": `.monaco/agents.local.toml:3: unknown key "batch.size"`,
		"[check.budget]\ngo = \"90s\"\n": `.monaco/agents.local.toml:2: unknown key "check.budget.go"`,
		"[check]\nslots = 0\n":           ".monaco/agents.local.toml: check.slots: want at least 1, got 0",
		"lanes = 0\n":                    ".monaco/agents.local.toml: lanes: want at least 1, got 0",
		"tracking = 0\n":                 ".monaco/agents.local.toml: tracking: want at least 1, got 0",
		"milestone = \"\"\n":             `.monaco/agents.local.toml: milestone: want a name, got ""`,
		"[dispatch]\nmax_load = 0\n":     ".monaco/agents.local.toml: dispatch.max_load: want above 0, got 0",
		"lanes = [\n":                    ".monaco/agents.local.toml:1: want a list",
	} {
		if _, err := applyLocalConfig(tracked, strings.NewReader(body)); err == nil ||
			!strings.Contains(cliText(err), want) {
			t.Errorf("%q: got %v, want %q", body, cliText(err), want)
		}
	}
	if tracked.Budget["go"] != defaultBudget()["go"] {
		t.Fatalf("a rejected budget line changed the tracked budget: %v", tracked.Budget["go"])
	}
}

func TestParseConfig_warnsOnceOnAKeyANewerConfigAdds(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	writeFile(t, filepath.Join(f.dir, configPath), testConfig+
		"[check]\nslots = 3\nqueue_hint = 1\n[check.budget]\nflows = \"90s\"\nkotlin = \"30s\"\nkotlin = \"40s\"\n")
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.noFailures()
	code, _, stderr := f.agents(t, "watch", "--once")
	want := `monacoctl agents: warning: unknown key "check.queue_hint" in .monaco/agents.toml (newer config, or a typo)` +
		"\n" + `monacoctl agents: warning: unknown key "check.budget.kotlin" in .monaco/agents.toml ` +
		"(newer config, or a typo)\n"
	if code != 0 || stderr != want {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	c, err := parseConfig(strings.NewReader(testConfig +
		"[check]\nslots = 3\nqueue_hint = 1\n[check.budget]\nflows = \"90s\"\nkotlin = \"30s\"\n"))
	if err != nil || c.Slots != 3 || c.Budget["flows"] != 90*time.Second || c.Batch != 2 ||
		!slices.Equal(c.Unknown, []string{"check.queue_hint", "check.budget.kotlin"}) {
		t.Fatalf("known values: %+v %v", c, err)
	}
}

func TestParseConfig_readsWatchStuckAfterAndDefaultsTo12m(t *testing.T) {
	t.Parallel()
	c, err := parseConfig(strings.NewReader(testConfig))
	if err != nil || c.StuckAfter != 12*time.Minute {
		t.Fatalf("default stuck_after = %s, %v", c.StuckAfter, err)
	}
	c, err = parseConfig(strings.NewReader(testConfig + "\n[watch]\nstuck_after = \"20m\"\n"))
	if err != nil || c.StuckAfter != 20*time.Minute {
		t.Fatalf("stuck_after = %s, %v", c.StuckAfter, err)
	}
	if _, err := parseConfig(strings.NewReader(testConfig + "\n[watch]\nstuck_after = \"0s\"\n")); err == nil ||
		!strings.Contains(cliText(err), "watch.stuck_after: want a positive duration") {
		t.Fatalf("zero stuck_after: %v", cliText(err))
	}
}
