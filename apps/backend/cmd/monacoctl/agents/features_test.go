package agents

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestOverrideBranch_prefersTheFlagThenTheEnvThenThePinAndRejectsOtherNames(t *testing.T) {
	t.Parallel()
	pinned := Config{FeatureBranch: "domain-core-checkpoint-2"}
	for _, tc := range []struct {
		flag, env string
		cfg       Config
		want, err string
	}{
		{"leaderboards-checkpoint-2", "following-checkpoint-5", pinned, "leaderboards-checkpoint-2", ""},
		{"", "following-checkpoint-5", pinned, "following-checkpoint-5", ""},
		{"", "", pinned, "domain-core-checkpoint-2", ""},
		{"", "", Config{FeatureBranch: autoFeatureBranch}, "", ""},
		{"", "", Config{FeatureBranch: "automation-checkpoint-1"}, "automation-checkpoint-1", ""},
		{"backend-rewrite-3", "", pinned, "", `--branch "backend-rewrite-3" is not a feature branch`},
		{"", "980-1-cloud-swift-633", pinned, "", `MONACO_FEATURE_BRANCH "980-1-cloud-swift-633" is not`},
		{"", "", Config{FeatureBranch: "fb"}, "", `feature_branch "fb" is not a feature branch`},
	} {
		got, err := overrideBranch(tc.flag, []string{featureBranchEnv + "=" + tc.env}, tc.cfg)
		if got != tc.want || (tc.err == "") != (err == nil) || (err != nil && !strings.Contains(cliText(err), tc.err)) {
			t.Errorf("%+v: got %q %v", tc, got, err)
		}
	}
}

func TestResolveHeader_movesAGoneBranchToItsFeaturesNewestCheckpoint(t *testing.T) {
	t.Parallel()
	live := []string{"backend-rewrite-checkpoint-4", "leaderboards-checkpoint-10", "leaderboards-checkpoint-9"}
	for named, want := range map[string]string{
		"backend-rewrite-checkpoint-4": "backend-rewrite-checkpoint-4",
		"backend-rewrite-3":            "backend-rewrite-checkpoint-4",
		"backend-rewrite-checkpoint-3": "backend-rewrite-checkpoint-4",
		"leaderboards-checkpoint-1":    "leaderboards-checkpoint-10",
	} {
		var notice bytes.Buffer
		got, err := resolveHeader(7, named, live, &notice)
		wantNotice := ""
		if named != want {
			wantNotice = "#7: Base branch " + named + " is gone; using " + want + "\n"
		}
		if err != nil || got != want || notice.String() != wantNotice {
			t.Errorf("%s: got %q %v, notice %q", named, got, err, notice.String())
		}
	}
	for named, want := range map[string]string{
		"main":                   `#7's Base branch "main" is not a feature branch`,
		"following-checkpoint-5": "#7's Base branch following-checkpoint-5 is gone, and origin has no following-checkpoint-<N>",
		"980-1-cloud-swift-633":  "is gone, and origin has no 980-1-cloud-swift-checkpoint-<N>",
	} {
		if got, err := resolveHeader(
			7,
			named,
			live,
			&bytes.Buffer{},
		); err == nil ||
			!strings.Contains(cliText(err), want) {
			t.Errorf("%s: got %q %v, want %q", named, got, err, want)
		}
	}
}

func fakeRepo(live, variable string, varErr error) Runner {
	return func(_ context.Context, _, _, name string, args ...string) ([]byte, error) {
		switch got := name + " " + strings.Join(args, " "); got {
		case "git ls-remote --heads origin *-checkpoint-*":
			return []byte(live), nil
		case "gh variable get FEATURE_BRANCH --repo o/r":
			return []byte(variable), varErr
		default:
			return nil, errors.New("unexpected " + got)
		}
	}
}

func TestTicketBranch_withoutAHeaderUsesTheRepoVariable(t *testing.T) {
	t.Parallel()
	env := &Env{Config: Config{Repo: "o/r"}, Run: fakeRepo("", "backend-rewrite-checkpoint-4\n", nil)}
	got, err := env.ticketBranch(t.Context(), 938, "**Milestone:** M8 · **Tracking:** #508", &bytes.Buffer{})
	if err != nil || got != "backend-rewrite-checkpoint-4" {
		t.Fatalf("got %q %v, want the FEATURE_BRANCH variable", got, err)
	}
	env.Run = fakeRepo("", "", errors.New("HTTP 404"))
	_, err = env.ticketBranch(t.Context(), 938, "", &bytes.Buffer{})
	if err == nil || !strings.Contains(cliText(err), "#938 has no **Base branch:** header") ||
		!strings.Contains(cliText(err), "HTTP 404") || !strings.Contains(cliText(err), "pass --branch") {
		t.Fatalf("got %v", err)
	}
	env.Branch = "leaderboards-checkpoint-2"
	if got, err := env.ticketBranch(t.Context(), 938, "**Base branch:** `following-checkpoint-5`", nil); err != nil ||
		got != "leaderboards-checkpoint-2" {
		t.Fatalf("override: got %q %v", got, err)
	}
}

func TestTrunks_listsEveryLiveFeatureBranchAndFallsBackToTheVariable(t *testing.T) {
	t.Parallel()
	live := "1\trefs/heads/leaderboards-checkpoint-2\n2\trefs/heads/following-checkpoint-5\n" +
		"3\trefs/heads/gh-readonly-queue/following-checkpoint-5/pr-1\n4\trefs/heads/x-checkpoint-1a\n"
	env := &Env{Config: Config{Repo: "o/r"}, Run: fakeRepo(live, "", errors.New("no variable"))}
	got, err := env.trunks(t.Context())
	if err != nil || strings.Join(got, " ") != "following-checkpoint-5 leaderboards-checkpoint-2" {
		t.Fatalf("got %q %v", got, err)
	}
	env.Run = fakeRepo("", "backend-rewrite-checkpoint-4\n", nil)
	if got, err := env.trunks(t.Context()); err != nil || strings.Join(got, " ") != "backend-rewrite-checkpoint-4" {
		t.Fatalf("fallback: got %q %v", got, err)
	}
	env.Run = fakeRepo("", "backend-rewrite-3\n", nil)
	if _, err := env.trunks(t.Context()); err == nil ||
		!strings.Contains(cliText(err), `"backend-rewrite-3" is not a feature branch`) {
		t.Fatalf("legacy variable: %v", err)
	}
}

func liveFixture(t *testing.T, branches ...string) *fixture {
	t.Helper()
	f := prepBranch(t)
	writeFile(t, filepath.Join(f.dir, configPath),
		strings.Replace(testConfig, "feature_branch = \"fb-checkpoint-1\"\n", "", 1))
	for _, b := range branches {
		git(t, f.dir, "branch", b)
	}
	return f
}

func (f *fixture) openStack(sha string, prs ...PR) {
	for i := range prs {
		prs[i].Head.SHA = sha
	}
	f.hub.on(list("/pulls?state=open"), prs)
	f.hub.on("GET /repos/o/r/commits/"+sha+"/check-runs?per_page=100", `{"check_runs":[]}`)
	f.hub.on(list("/commits/"+sha+"/statuses?"), []GHStatus{})
}

func TestStatus_listsStacksUnderEachLiveFeatureBranch(t *testing.T) {
	t.Parallel()
	f := liveFixture(t, "leaderboards-checkpoint-2", "following-checkpoint-5")
	f.openStack(strings.Repeat("c", 40),
		pr(1, "l1", "leaderboards-checkpoint-2", ""), pr(2, "f1", "following-checkpoint-5", ""),
		pr(3, "l2", "l1", ""), pr(4, "o1", "gone", ""))
	code, stdout, stderr := f.agents(t, "status")
	want := "| #2 | following-checkpoint-5 | ccccccc | none | none | none |\n" +
		"| #1 | leaderboards-checkpoint-2 | ccccccc | none | none | none |\n" +
		"| #3 | leaderboards-checkpoint-2 | ccccccc | none | none | none |\n"
	if code != 0 || !strings.HasSuffix(stdout, "| verify |\n"+want) {
		t.Fatalf("code=%d stderr=%q stdout:\n%s", code, stderr, stdout)
	}
}

func TestStatus_listsAnM8StackWhoseChildrenSitOnTicketBranches(t *testing.T) {
	t.Parallel()
	f := liveFixture(t, "backend-rewrite-checkpoint-4")
	f.openStack(strings.Repeat("d", 40),
		pr(11, "980-1-cloud-swift-633", "backend-rewrite-checkpoint-4", "Part of #980"),
		pr(12, "980-2-cloud-swift-634", "980-1-cloud-swift-633", "Part of #980"),
		pr(13, "980-3-cloud-swift-635", "980-2-cloud-swift-634", "Closes #980"))
	code, stdout, stderr := f.agents(t, "status")
	for _, n := range []string{"11", "12", "13"} {
		if !strings.Contains(stdout, "| #"+n+" | backend-rewrite-checkpoint-4 | ddddddd |") {
			t.Errorf("#%s missing: code=%d stderr=%q stdout:\n%s", n, code, stderr, stdout)
		}
	}
}

func (f *fixture) dispatchDry(t *testing.T, ticket int, body string) (int, string, string) {
	t.Helper()
	n := strconv.Itoa(ticket)
	f.batch(t, ticket)
	f.hub.on(get("/issues/"+n), Issue{Number: ticket, Body: body})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.ps()
	return f.agents(t, "dispatch", n, "--model", "opus", "--dry-run")
}

func (f *fixture) tip(t *testing.T, branch string) string {
	t.Helper()
	out, err := Exec(t.Context(), f.dir, "", "git", "rev-parse", branch)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestDispatch_startsTheWorktreeFromTheTicketsBaseBranch(t *testing.T) {
	t.Parallel()
	f := liveFixture(t, "following-checkpoint-5")
	git(t, f.dir, "checkout", "-q", "-b", "leaderboards-checkpoint-2")
	tip := commitFile(t, f.dir, "l.go", "package l\n")
	code, stdout, stderr := f.dispatchDry(t, 21, "**Base branch:** `leaderboards-checkpoint-2` · **Touches:** `a`")
	if code != 0 || !strings.Contains(stdout, "would add worktree "+f.Env(t).worktreePath(21)+" at "+tip+"\n") ||
		!strings.Contains(stdout, "parent: "+tip+"\n") {
		t.Fatalf("code=%d stderr=%q stdout:\n%s", code, stderr, stdout)
	}
}

func TestDispatch_movesAStaleBackendRewriteHeaderToTheLiveCheckpoint(t *testing.T) {
	t.Parallel()
	f := liveFixture(t, "backend-rewrite-checkpoint-4")
	code, stdout, stderr := f.dispatchDry(t, 22, "**Base branch:** `backend-rewrite-3` · **Touches:** `a`")
	if code != 0 ||
		!strings.HasPrefix(
			stdout,
			"#22: Base branch backend-rewrite-3 is gone; using backend-rewrite-checkpoint-4\n",
		) ||
		!strings.Contains(
			stdout,
			"no file is touched by more than one open stack into backend-rewrite-checkpoint-4\n",
		) ||
		!strings.Contains(stdout, " at "+f.tip(t, "backend-rewrite-checkpoint-4")+"\n") {
		t.Fatalf("code=%d stderr=%q stdout:\n%s", code, stderr, stdout)
	}
}

func TestDispatch_withoutAHeaderUsesTheFeatureBranchVariable(t *testing.T) {
	t.Parallel()
	f := liveFixture(t, "backend-rewrite-checkpoint-4")
	prev := f.run
	f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "gh" && strings.Join(args, " ") == "variable get FEATURE_BRANCH --repo o/r" {
			return []byte("fb-checkpoint-1\n"), nil
		}
		return prev(ctx, dir, stdin, name, args...)
	}
	code, stdout, stderr := f.dispatchDry(t, 23, "**Milestone:** M8 · **Touches:** `a`")
	if code != 0 || !strings.Contains(stdout, "into fb-checkpoint-1\n") ||
		!strings.Contains(stdout, " at "+f.tip(t, "fb-checkpoint-1")+"\n") {
		t.Fatalf("code=%d stderr=%q stdout:\n%s", code, stderr, stdout)
	}
}

func TestRunCLI_rejectsABranchFlagWithoutAValue(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if code, _, stderr := f.agents(t, "status", "--branch"); code != 2 || !strings.Contains(stderr, "usage:") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestWalkStack_stopsAtWhicheverFeatureBranchTheStackLandsOn(t *testing.T) {
	t.Parallel()
	open := []stackPR{
		{gqlPR: gqlPR{Number: 1}, Head: "l1", Base: "leaderboards-checkpoint-2"},
		{gqlPR: gqlPR{Number: 2}, Head: "l2", Base: "l1"},
		{gqlPR: gqlPR{Number: 3}, Head: "f1", Base: "following-checkpoint-5"},
	}
	trunks := []string{"following-checkpoint-5", "leaderboards-checkpoint-2"}
	for top, want := range map[int]string{2: "#1 #2 on leaderboards-checkpoint-2", 3: "#3 on following-checkpoint-5"} {
		stack, err := walkStack(open, top, trunks)
		var nums []int
		for _, p := range stack {
			nums = append(nums, p.Number)
		}
		if err != nil || prList(nums)+" on "+stack[0].Base != want {
			t.Errorf("top #%d: got %v %v, want %s", top, nums, err, want)
		}
	}
}
