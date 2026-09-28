package agents

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDispatch_dryRunWritesNothing(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.hub.on(get("/issues/12"), Issue{Number: 12, Body: "Blocked by #3"})
	merged := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	f.hub.on(get("/issues/3"), Issue{Number: 3, PullRequest: &struct{}{}})
	f.hub.on(get("/pulls/3"), PR{Number: 3, MergedAt: &merged, MergeCommitSHA: f.head(t)})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.ps("1 claude\n")
	code, stdout, stderr := f.agents(t, "dispatch", "12", "--model", "opus", "--dry-run")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "dry-run: would add worktree") ||
		!strings.Contains(stdout, "would start caffeinate") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(f.Env(t).recordPath(12)); !os.IsNotExist(err) {
		t.Fatalf("record written: %v", err)
	}
}

func TestDispatch_refusesBlockersLanesAndModel(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	if code, _, stderr := f.agents(t, "dispatch"); code != 2 || !strings.Contains(stderr, "dispatch <ticket>") {
		t.Fatalf("usage: %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(
		t,
		"dispatch",
		"4",
		"--model",
		"fable",
	); code != 1 ||
		!strings.Contains(stderr, "fable") {
		t.Fatalf("fable: %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(
		t,
		"dispatch",
		"4",
		"--model",
		"opus",
	); code != 1 ||
		!strings.Contains(stderr, "issues/4") {
		t.Fatalf("issue: %d %q", code, stderr)
	}
	f.hub.on(get("/issues/4"), Issue{Body: "Blocked by nothing"})
	if code, _, stderr := f.agents(
		t,
		"dispatch",
		"4",
		"--model",
		"opus",
	); code != 1 ||
		!strings.Contains(stderr, "no issue numbers") {
		t.Fatalf("empty: %d %q", code, stderr)
	}
	f.hub.on(get("/issues/4"), Issue{Body: "Blocked by #8"})
	f.hub.on(get("/issues/8"), Issue{State: "open"})
	f.hub.on(list("/pulls?state=closed"), []PR{{Number: 1, Body: "nope", Base: Ref{Ref: "fb"}}})
	if code, _, stderr := f.agents(
		t,
		"dispatch",
		"4",
		"--model",
		"opus",
	); code != 1 ||
		!strings.Contains(stderr, "not merged into") {
		t.Fatalf("open: %d %q", code, stderr)
	}
	f.hub.on(get("/issues/8"), Issue{State: "closed", StateReason: "completed"})
	f.owner(t, Record{Ticket: 1, State: Running})
	f.owner(t, Record{Ticket: 2, State: Done})
	if code, _, stderr := f.agents(
		t,
		"dispatch",
		"4",
		"--model",
		"opus",
	); code != 1 ||
		!strings.Contains(stderr, "lane cap") {
		t.Fatalf("lanes: %d %q", code, stderr)
	}
}

func TestDispatch_startsAWorktreeWhenTheBlockerIsInTheBranch(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	tip := f.head(t)
	f.hub.on(get("/issues/12"), Issue{Body: "no blockers"})
	f.hub.on(list("/pulls?state=open"), []PR{pr(1, "a", "fb", ""), pr(2, "b", "fb", "")})
	f.hub.on(list("/pulls/1/files?"), []File{{Filename: "shared.go"}})
	f.hub.on(list("/pulls/2/files?"), []File{{Filename: "shared.go"}})
	f.ps("1 claude\n")
	var started []string
	env := f.Env(t)
	env.Start = func(name string, args ...string) error {
		started = append(started, name+" "+strings.Join(args, " "))
		return nil
	}
	env.Run = f.run
	var stdout strings.Builder
	if err := dispatchCmd(context.Background(), env, []string{"12", "--model", "opus"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 || !strings.HasPrefix(started[0], "caffeinate -dimsu -w ") {
		t.Fatalf("started=%v", started)
	}
	rec, err := env.record(12)
	if err != nil || rec.Model != opus || rec.State != Running || rec.Base != tip {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
	if _, err := os.Stat(rec.Worktree); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "shared.go") {
		t.Fatal(stdout.String())
	}
}

func TestDispatch_acceptsAClosedIssueAndAMergedPull(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	merged := f.now
	f.hub.on(get("/issues/12"), Issue{Body: "Blocked by #8, #9"})
	f.hub.on(get("/issues/8"), Issue{State: "closed", StateReason: "completed"})
	f.hub.on(get("/issues/9"), Issue{PullRequest: &struct{}{}})
	f.hub.on(get("/pulls/9"), PR{MergedAt: &merged, MergeCommitSHA: f.head(t)})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.ps("1 claude\n")
	env := f.Env(t)
	env.Run = f.run
	env.Start = func(string, ...string) error { return failure("caffeinate down") }
	if err := dispatchCmd(
		context.Background(),
		env,
		[]string{"12", "--model", "sonnet"},
		ioDiscard(),
	); err == nil ||
		!strings.Contains(err.Error(), "caffeinate down") {
		t.Fatal(err)
	}
	f.hub.on(get("/pulls/9"), PR{MergeCommitSHA: "dead"})
	if err := dispatchCmd(context.Background(), env, []string{"12", "--model", "sonnet"}, ioDiscard()); err == nil {
		t.Fatal("expected unmerged or missing sha")
	}
}

func TestConflicts_printsTheRebaseTask(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	base := f.head(t)
	git(t, f.dir, "checkout", "-q", "-b", "side")
	left := commitFile(t, f.dir, "c.go", "left\n")
	git(t, f.dir, "checkout", "-q", "fb")
	_ = commitFile(t, f.dir, "c.go", "right\n")
	git(t, f.dir, "update-ref", "refs/remotes/origin/fb", "fb")
	f.owner(t, Record{Ticket: 40, Worktree: f.dir, AgentID: "agt"})
	f.hub.on(get("/pulls/5"), headed(5, left))
	code, stdout, stderr := f.agents(t, "conflicts", "5")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "behind: yes") ||
		!strings.Contains(stdout, "file: c.go") ||
		!strings.Contains(stdout, "agent: agt") ||
		!strings.Contains(stdout, "gt sync --no-interactive then gt restack") {
		t.Fatalf("code=%d stdout=%q stderr=%q base=%s", code, stdout, stderr, base)
	}
}

func TestOwnDoneExited(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.owner(t, Record{Ticket: 4, State: Running, Model: opus})
	if code, _, stderr := f.agents(t, "own", "4", "agt"); code != 0 || stderr != "" {
		t.Fatalf("own: %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "done", "4"); code != 0 {
		t.Fatalf("done: %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "exited", "4"); code != 0 {
		t.Fatalf("exited: %d %q", code, stderr)
	}
	rec, err := f.Env(t).record(4)
	if err != nil || rec.AgentID != "agt" || rec.State != Exited {
		t.Fatalf("%+v %v", rec, err)
	}
	if code, _, stderr := f.agents(t, "own", "4"); code != 2 {
		t.Fatalf("usage: %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "done", "9"); code != 1 || !strings.Contains(stderr, "no owner record") {
		t.Fatalf("missing: %d %q", code, stderr)
	}
}

func (f *fixture) ps(out string) {
	prev := f.run
	f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "ps" {
			return []byte(out), nil
		}
		if name == "pgrep" {
			return nil, failure("none")
		}
		return prev(ctx, dir, stdin, name, args...)
	}
}

func prepBranch(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	git(t, f.dir, "commit", "-q", "--allow-empty", "-m", "root")
	git(t, f.dir, "branch", "fb")
	return f
}

func (f *fixture) head(t *testing.T) string {
	t.Helper()
	out, err := Exec(context.Background(), f.dir, "", "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func ioDiscard() *discard { return &discard{} }
