package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDispatch_dryRunWritesNothing(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.batch(t, 12)
	f.hub.on(get("/issues/12"), Issue{Number: 12, Body: "**Milestone:** M7 · **Blocked by:** #3 · **Touches:** `a`"})
	merged := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	f.hub.on(get("/issues/3"), Issue{Number: 3, PullRequest: &struct{}{}})
	f.hub.on(get("/pulls/3"), PR{Number: 3, MergedAt: &merged, MergeCommitSHA: f.head(t)})
	f.hub.on(list("/pulls?state=open"), []PR{pr(1, "a", "fb-checkpoint-1", ""), pr(2, "b", "fb-checkpoint-1", "")})
	shared := make([]File, 0, 30)
	for i := range 30 {
		shared = append(shared, File{Filename: fmt.Sprintf("shared%02d.go", i)})
	}
	f.hub.on(list("/pulls/1/files?"), shared)
	f.hub.on(list("/pulls/2/files?"), shared)
	f.ps()
	code, stdout, stderr := f.agents(t, "dispatch", "12", "--model", "opus", "--dry-run")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "dry-run: would add worktree") ||
		!strings.Contains(stdout, "would start caffeinate") ||
		!strings.Contains(stdout, "subagent_type=pstack:poteto-agent model=opus run_in_background=true, prompt:\n"+
			"ticket: 12\nworktree: ") || !strings.Contains(stdout, "dry-run: would post the owner record on #12\n") ||
		!strings.Contains(stdout, "brief: docs/agents/owner.md\norders: docs/agents/standing-orders.md\n30 files") ||
		!strings.HasSuffix(stdout, " more\n") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(f.Env(t).recordPath(12)); !os.IsNotExist(err) {
		t.Fatalf("record written: %v", err)
	}
}

func TestDispatch_refusesBlockersLanesAndModel(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.batch(t, 4)
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
		!strings.Contains(stderr, `must be opus or sonnet, got "fable"`) {
		t.Fatalf("fable: %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "dispatch", "4", "--model", "haiku"); code != 1 ||
		!strings.Contains(stderr, `got "haiku"`) {
		t.Fatalf("haiku: %d %q", code, stderr)
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
	f.hub.on(get("/issues/4"), Issue{Body: "**Milestone:** M7 · **Blocked by:** the retro"})
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
	f.hub.on(get("/issues/4"), Issue{Body: "**Milestone:** M7 · **Blocked by:** #8 · **Touches:** `a`"})
	f.hub.on(get("/issues/8"), Issue{State: "open"})
	f.hub.on(list("/pulls?state=closed"), []PR{})
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
	f.batch(t, 12)
	tip := f.head(t)
	f.hub.on(get("/issues/12"), Issue{Body: "no blockers"})
	f.hub.on(list("/pulls?state=open"), []PR{pr(1, "a", "fb-checkpoint-1", ""), pr(2, "b", "fb-checkpoint-1", "")})
	f.hub.on(list("/pulls/1/files?"), []File{{Filename: "shared.go"}})
	f.hub.on(list("/pulls/2/files?"), []File{{Filename: "shared.go"}})
	f.ownerComments(12)
	f.ps()
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
	rec, err := env.localRecord(12)
	if err != nil || rec.Model != opus || rec.State != Running || rec.Base != tip {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
	if _, err := os.Stat(rec.Worktree); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "shared.go") {
		t.Fatal(stdout.String())
	}
	spawn := "spawn: Agent subagent_type=pstack:poteto-agent model=opus run_in_background=true, prompt:\n" +
		"ticket: 12\nworktree: " + rec.Worktree + "\nparent: " + tip + "\nbrief: docs/agents/owner.md\n" +
		"orders: docs/agents/standing-orders.md\n"
	if !strings.HasPrefix(stdout.String(), spawn) {
		t.Fatalf("stdout=%q want prefix %q", stdout.String(), spawn)
	}
	posted := posted(t, f, "POST /repos/o/r/issues/12/comments")
	if !strings.HasPrefix(posted, recordMarker+"\nOwner record for #12: model opus, state running, branch none yet") ||
		strings.Contains(posted, rec.Worktree) || strings.Contains(posted, "worktree") {
		t.Fatalf("record comment %q", posted)
	}
}

func TestDispatch_startsFromOriginWhenTheLocalBranchIsBehind(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.batch(t, 12)
	stale := f.head(t)
	ahead := commitFile(t, f.dir, "ahead.go", "x\n")
	f.hub.on(get("/issues/12"), Issue{Body: "**Milestone:** M7 · **Blocked by:** none · **Touches:** `a`"})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.ownerComments(12)
	f.ps()
	env := f.Env(t)
	var fetched []string
	fetchErr := errors.New("offline")
	target := ""
	env.Run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "git" && len(args) > 0 && args[0] == "fetch" {
			fetched = append(fetched, strings.Join(args, " "))
			if fetchErr != nil {
				return nil, fetchErr
			}
			if target == "" {
				return Exec(ctx, f.dir, "", "git", "update-ref", "-d", "refs/remotes/origin/fb-checkpoint-1")
			}
			return Exec(ctx, f.dir, "", "git", "update-ref", "refs/remotes/origin/fb-checkpoint-1", target)
		}
		return f.run(ctx, dir, stdin, name, args...)
	}
	args := []string{"12", "--model", "opus"}
	if err := dispatchCmd(context.Background(), env, args, ioDiscard()); !errors.Is(err, fetchErr) {
		t.Fatalf("fetch: %v", err)
	}
	fetchErr = nil
	if err := dispatchCmd(context.Background(), env, []string{"12", "--model", "opus"}, ioDiscard()); err == nil {
		t.Fatal("expected a missing origin ref")
	}
	target = ahead
	if err := dispatchCmd(context.Background(), env, []string{"12", "--model", "opus"}, ioDiscard()); err != nil {
		t.Fatal(err)
	}
	if len(fetched) != 3 || fetched[2] != "fetch origin fb-checkpoint-1" {
		t.Fatalf("fetched=%q", fetched)
	}
	rec, err := env.localRecord(12)
	if err != nil || rec.Base != ahead || rec.Base == stale {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
	out, err := Exec(context.Background(), rec.Worktree, "", "git", "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(string(out)) != ahead {
		t.Fatalf("worktree head %q err=%v", out, err)
	}
	if local := strings.TrimSpace(gitOut(t, f.dir, "rev-parse", "fb-checkpoint-1")); local != stale {
		t.Fatalf("local fb-checkpoint-1 moved to %s", local)
	}
}

func TestDispatch_acceptsAClosedIssueAndAMergedPull(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.batch(t, 12)
	merged := f.now
	f.hub.on(get("/issues/12"), Issue{Body: "**Blocked by:** #8, #9"})
	f.hub.on(get("/issues/8"), Issue{State: "closed", StateReason: "completed"})
	f.hub.on(get("/issues/9"), Issue{PullRequest: &struct{}{}})
	f.hub.on(get("/pulls/9"), PR{MergedAt: &merged, MergeCommitSHA: f.head(t)})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.ps()
	env := f.Env(t)
	env.Run = f.run
	env.Start = func(string, ...string) error { return errors.New("caffeinate down") }
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

func TestWatch_idleAliveAndCaffeinate(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	old := f.now.Add(-time.Hour)
	f.owner(t, Record{Ticket: 1, State: Running, Started: old, Worktree: f.dir})
	f.owner(t, Record{Ticket: 2, State: Done, Worktree: f.dir})
	f.owner(t, Record{Ticket: 3, State: Exited, Started: old})
	f.hub.on(get("/issues/1"), Issue{UpdatedAt: old})
	f.hub.on(list("/pulls?state=open"), []PR{{Number: 4, Body: "Part of #1", UpdatedAt: old}})
	f.watchGit("HEAD", "1", "")
	f.noFailures()
	code, stdout, stderr := f.agents(t, "watch")
	if code != 1 || stderr != "" || !strings.Contains(stdout, "idle: #1") ||
		!strings.Contains(stdout, "done but alive: #2") ||
		!strings.Contains(stdout, "missing caffeinate") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if code, _, stderr := f.agents(t, "watch", "x"); code != 2 {
		t.Fatalf("usage: %d %q", code, stderr)
	}
}

func TestWatch_isQuietWhenEveryoneMoved(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.owner(t, Record{Ticket: 1, State: Running, Started: f.now, Worktree: f.dir})
	f.owner(t, Record{Ticket: 2, State: Done, Worktree: filepath.Join(f.dir, "missing")})
	f.hub.on(get("/issues/1"), Issue{UpdatedAt: f.now})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.watchGit("topic", strconv.FormatInt(f.now.Unix(), 10), "pgrep")
	f.noFailures()
	code, stdout, stderr := f.agents(t, "watch")
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestConflicts_printsTheRebaseTask(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	base := f.head(t)
	git(t, f.dir, "checkout", "-q", "-b", "side")
	left := commitFile(t, f.dir, "c.go", "left\n")
	git(t, f.dir, "checkout", "-q", "fb-checkpoint-1")
	_ = commitFile(t, f.dir, "c.go", "right\n")
	git(t, f.dir, "update-ref", "refs/remotes/origin/fb-checkpoint-1", "fb-checkpoint-1")
	f.owner(t, Record{Ticket: 40, Worktree: f.dir, AgentID: "agt"})
	f.hub.on(get("/pulls/5"), headed(5, left))
	code, stdout, stderr := f.agents(t, "conflicts", "5")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "behind: yes") ||
		!strings.Contains(stdout, "file: c.go") ||
		!strings.Contains(stdout, "agent: agt") ||
		!strings.Contains(stdout, "gt sync --no-interactive --no-restack, run gt restack, monacoctl agents check") {
		t.Fatalf("code=%d stdout=%q stderr=%q base=%s", code, stdout, stderr, base)
	}
}

func TestStatus_publishesAndSkipsAnUnchangedComment(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	sha := strings.Repeat("a", 40)
	f.hub.on(list("/pulls?state=open"), []PR{headed(5, sha)})
	f.hub.on(
		"GET /repos/o/r/commits/"+sha+"/check-runs?per_page=100",
		`{"check_runs":[{"name":"ci","conclusion":"success"},{"name":"ci / ci-ok","status":"queued"}]}`,
	)
	f.hub.on(
		list("/commits/"+sha+"/statuses?"),
		[]GHStatus{{Context: "verify", State: "success"}, {Context: "other", State: "failure"}},
	)
	f.hub.on(list("/issues/7/comments?"), []Comment{})
	f.hub.on("POST /repos/o/r/issues/7/comments", "ok")
	code, stdout, stderr := f.agents(t, "status", "--publish")
	if code != 0 || stdout != "status comment updated\n" || stderr != "" ||
		!strings.Contains(f.hub.body("POST /repos/o/r/issues/7/comments"), "monacoctl agents status") {
		t.Fatalf(
			"code=%d stdout=%q stderr=%q body=%s",
			code,
			stdout,
			stderr,
			f.hub.body("POST /repos/o/r/issues/7/comments"),
		)
	}
	plain := statusMarker + "\n| pr | sha | ci | ci-ok | verify |\n| #5 | aaaaaaa | success | queued | success |\n"
	f.hub.on(list("/issues/7/comments?"), []Comment{authored(9, plain, ghUser, "MEMBER")})
	code, stdout, stderr = f.agents(t, "status", "--publish")
	if code != 0 || stdout != "status comment unchanged\n" {
		t.Fatalf("same: %d %q %q", code, stdout, stderr)
	}
	f.hub.on(list("/issues/7/comments?"), []Comment{authored(9, statusMarker+"\nstale", ghUser, "MEMBER")})
	f.hub.on("PATCH /repos/o/r/issues/comments/9", "ok")
	if code, stdout, stderr = f.agents(t, "status", "--publish"); code != 0 || stdout != "status comment updated\n" ||
		posted(t, f, "PATCH /repos/o/r/issues/comments/9") != plain {
		t.Fatalf("patch: %d %q %q", code, stdout, stderr)
	}
	if code, _, stderr := f.agents(t, "status"); code != 2 {
		t.Fatalf("usage: %d %q", code, stderr)
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
	if code, _, stderr := f.agents(t, "exited", "4"); code != 0 ||
		!strings.Contains(posted(t, f, "POST /repos/o/r/issues/4/comments"), `"state":"exited"`) {
		t.Fatalf("exited: %d %q", code, stderr)
	}
	rec, err := f.Env(t).localRecord(4)
	if err != nil || rec.AgentID != "agt" || rec.State != Exited {
		t.Fatalf("%+v %v", rec, err)
	}
	if code, _, stderr := f.agents(t, "own", "4"); code != 2 {
		t.Fatalf("usage: %d %q", code, stderr)
	}
	f.ownerComments(9)
	if code, _, stderr := f.agents(t, "done", "9"); code != 1 || !strings.Contains(stderr, "no owner record") {
		t.Fatalf("missing: %d %q", code, stderr)
	}
}

func (f *fixture) ps() {
	prev := f.run
	f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "ps" {
			return []byte("1 claude\n"), nil
		}
		if name == "pgrep" {
			return nil, errors.New("none")
		}
		return prev(ctx, dir, stdin, name, args...)
	}
}

func (f *fixture) watchGit(branch, stamp, alive string) {
	prev := f.run
	f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		switch name {
		case "pgrep":
			if alive == "pgrep" {
				return []byte("1\n"), nil
			}
			return nil, errors.New("none")
		case "lsof":
			if alive == "pgrep" {
				return []byte("n/somewhere/else\n"), nil
			}
			return []byte("n" + f.dir + "\n"), nil
		case "git":
			if len(args) > 2 && args[1] == "--abbrev-ref" {
				return []byte(branch + "\n"), nil
			}
			if len(args) > 0 && args[0] == "log" {
				if len(args) > 3 && args[3] == "origin/"+branch {
					return nil, errors.New("no origin")
				}
				return []byte(stamp + "\n"), nil
			}
		}
		return prev(ctx, dir, stdin, name, args...)
	}
}

func prepBranch(t *testing.T) *fixture {
	t.Helper()
	f := newFixtureFrom(t, rootedRepo)
	git(t, f.dir, "remote", "add", "origin", f.dir)
	git(t, f.dir, "update-ref", "refs/remotes/origin/fb-checkpoint-1", "fb-checkpoint-1")
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
