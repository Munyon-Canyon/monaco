package agents

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestEdges_dispatchBlockersAndProcess(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	env := f.Env(t)
	env.Run = f.run
	f.hub.on(get("/issues/4"), Issue{Body: "Blocked by #8"})
	if err := env.blockersClear(context.Background(), 4); err == nil || !strings.Contains(err.Error(), "issues/8") {
		t.Fatal(err)
	}
	f.hub.on(get("/issues/8"), Issue{PullRequest: &struct{}{}})
	if err := env.blockersClear(context.Background(), 4); err == nil || !strings.Contains(err.Error(), "pulls/8") {
		t.Fatal(err)
	}
	f.hub.on(get("/pulls/8"), PR{})
	if err := env.blockersClear(context.Background(), 4); err == nil || !strings.Contains(err.Error(), "not merged") {
		t.Fatal(err)
	}
	when := f.now
	side := commitFile(t, f.dir, "side.go", "x\n")
	git(t, f.dir, "update-ref", "refs/heads/fb", "HEAD~1")
	f.hub.on(get("/pulls/8"), PR{MergedAt: &when, MergeCommitSHA: side})
	if err := env.blockersClear(context.Background(), 4); err == nil || !strings.Contains(err.Error(), "not in") {
		t.Fatal(err)
	}
	f.hub.on(get("/pulls/8"), PR{MergedAt: &when, MergeCommitSHA: "not-a-sha"})
	if err := env.blockersClear(context.Background(), 4); err == nil {
		t.Fatal("bad sha")
	}
	f.hub.on(get("/issues/8"), Issue{State: "open"})
	if err := env.blockersClear(
		context.Background(),
		4,
	); err == nil ||
		!strings.Contains(err.Error(), "pulls?state=closed") {
		t.Fatal(err)
	}
	f.hub.on(
		list("/pulls?state=closed"),
		[]PR{
			{MergedAt: &when, Base: Ref{Ref: "other"}, Body: "no"},
			{MergedAt: &when, Base: Ref{Ref: "fb"}, Body: "Closes #8", MergeCommitSHA: side},
		},
	)
	git(t, f.dir, "update-ref", "refs/heads/fb", "HEAD")
	if err := env.blockersClear(context.Background(), 4); err != nil {
		t.Fatal(err)
	}
	git(t, f.dir, "update-ref", "refs/remotes/origin/fb", "HEAD")
	git(t, f.dir, "branch", "-D", "fb")
	if _, err := env.ancestor(context.Background(), side); err != nil {
		t.Fatal(err)
	}
	git(t, f.dir, "update-ref", "-d", "refs/remotes/origin/fb")
	if _, err := env.featureRef(context.Background()); err == nil {
		t.Fatal("missing ref")
	}
	git(t, f.dir, "branch", "fb")
	writeFile(t, env.Common+"/.monaco/agents", "file")
	if err := env.lanesOpen(); err == nil {
		t.Fatal("lanes")
	}
	env.Run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "ps" {
			return nil, failure("ps down")
		}
		return f.run(ctx, dir, stdin, name, args...)
	}
	if _, err := env.claudePID(context.Background()); err == nil || !strings.Contains(err.Error(), "ps down") {
		t.Fatal(err)
	}
	for _, out := range []string{"only\n", "x comm\n", "1 launchd\n", "2 other\n"} {
		env.Run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
			if name == "ps" {
				return []byte(out), nil
			}
			return f.run(ctx, dir, stdin, name, args...)
		}
		if _, err := env.claudePID(
			context.Background(),
		); err == nil ||
			!strings.Contains(err.Error(), "missing assertion") {
			t.Fatalf("%q: %v", out, err)
		}
	}
	env.Run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "pgrep" {
			return []byte("4\n"), nil
		}
		return f.run(ctx, dir, stdin, name, args...)
	}
	if note, err := env.caffeinePlan(
		context.Background(),
		true,
	); err != nil ||
		!strings.Contains(note, "already running") {
		t.Fatal(note, err)
	}
	if _, err := positiveInt("nope", "dispatch <ticket>"); err == nil {
		t.Fatal("int")
	}
	if err := setState(env, []string{"x"}, Done, "done <ticket>"); err == nil {
		t.Fatal("state")
	}
	if err := setState(env, nil, Done, "done <ticket>"); err == nil {
		t.Fatal("usage")
	}
}

func TestEdges_watchConflictsStatus(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	if err := watchCmd(context.Background(), env, []string{"x"}, ioDiscard()); err == nil {
		t.Fatal("usage")
	}
	writeFile(t, env.Common+"/.monaco/agents", "file")
	if err := watchCmd(context.Background(), env, nil, ioDiscard()); err == nil {
		t.Fatal("records")
	}
	if err := os.Remove(env.Common + "/.monaco/agents"); err != nil {
		t.Fatal(err)
	}
	f.owner(t, Record{Ticket: 1, State: Running, Started: f.now, Worktree: f.dir})
	env = f.Env(t)
	env.Run = func(context.Context, string, string, string, ...string) ([]byte, error) {
		return nil, failure("git down")
	}
	if _, err := env.lastActivity(
		context.Background(),
		Record{Ticket: 1, Worktree: f.dir, Started: f.now},
	); err == nil {
		t.Fatal("commit")
	}
	env.Run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "git" && len(args) > 0 && args[0] == "log" {
			return []byte("nope\n"), nil
		}
		if name == "git" {
			return []byte("topic\n"), nil
		}
		return nil, failure("down")
	}
	if _, err := env.lastActivity(
		context.Background(),
		Record{Ticket: 1, Worktree: f.dir},
	); err == nil ||
		!strings.Contains(err.Error(), "commit time") {
		t.Fatal(err)
	}
	env.Run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "git" && len(args) > 3 {
			return []byte("nope\n"), nil
		}
		if name == "git" && len(args) > 0 && args[0] == "log" {
			return []byte("10\n"), nil
		}
		if name == "git" {
			return []byte("topic\n"), nil
		}
		return nil, failure("down")
	}
	f.hub.on(get("/issues/1"), Issue{UpdatedAt: f.now.Add(time.Hour)})
	later := f.now.Add(2 * time.Hour)
	f.hub.on(list("/pulls?state=open"), []PR{{Body: "see #1", UpdatedAt: later}})
	if _, err := env.lastActivity(
		context.Background(),
		Record{Ticket: 1, Started: f.now, Worktree: f.dir},
	); err == nil ||
		!strings.Contains(err.Error(), "commit time") {
		t.Fatal(err)
	}
	if _, err := env.alive(context.Background(), f.dir); err == nil {
		t.Fatal("lsof")
	}
	var buf strings.Builder
	files := make([]string, 20)
	for i := range files {
		files[i] = "f"
	}
	if err := writeRebase(
		&buf,
		pr(1, "h", "fb", ""),
		Record{Worktree: "w", AgentID: "a"},
		false,
		files,
	); err != nil ||
		!strings.Contains(buf.String(), "and 5 more") {
		t.Fatal(buf.String(), err)
	}
	if code, _, stderr := f.agents(t, "conflicts"); code != 2 {
		t.Fatalf("usage %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "conflicts", "5"); code != 1 {
		t.Fatalf("missing %d %q", code, stderr)
	}
	f.hub.on(get("/pulls/5"), headed(5, "missing"))
	if code, _, stderr := f.agents(t, "conflicts", "5"); code != 1 {
		t.Fatalf("sha %d %q", code, stderr)
	}
	g := newFixture(t)
	if _, err := g.Env(t).statusBody(context.Background()); err == nil {
		t.Fatal("status prs")
	}
	sha := strings.Repeat("b", 40)
	g.hub.on(list("/pulls?state=open"), []PR{headed(2, sha)})
	if _, err := g.Env(t).statusBody(context.Background()); err == nil || !strings.Contains(err.Error(), "check-runs") {
		t.Fatal(err)
	}
	g.hub.on(
		"GET /repos/o/r/commits/"+sha+"/check-runs?per_page=100",
		`{"check_runs":[{"name":"other","conclusion":"success"}]}`,
	)
	if _, err := g.Env(t).statusBody(context.Background()); err == nil || !strings.Contains(err.Error(), "statuses") {
		t.Fatal(err)
	}
	g.hub.on(list("/commits/"+sha+"/statuses?"), []GHStatus{{Context: "other", State: "pending"}})
	body, err := g.Env(t).statusBody(context.Background())
	if err != nil || !strings.Contains(body, "| none | none | none |") {
		t.Fatal(body, err)
	}
	rows := make([]PR, maxLines)
	for i := range rows {
		rows[i] = pr(i+1, fmt.Sprintf("h%d", i), "fb", "")
		rows[i].Head.SHA = sha
	}
	g.hub.on(list("/pulls?state=open"), rows)
	body, err = g.Env(t).statusBody(context.Background())
	if err != nil || !strings.Contains(body, "and ") {
		t.Fatal(body, err)
	}
	if _, _, err := g.Env(t).statusComment(context.Background()); err == nil {
		t.Fatal("comments")
	}
	if code, _, stderr := g.agents(t, "status", "--publish"); code != 1 {
		t.Fatalf("status cmd %d %q", code, stderr)
	}
}

func TestEdges_remainingBranches(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	env := f.Env(t)
	if !closes("Closes #2", 2) || closes("nope", 2) || closes("Closes #1", 2) {
		t.Fatal("closes")
	}
	git(t, f.dir, "branch", "-D", "fb")
	if _, err := env.featureTip(context.Background()); err == nil {
		t.Fatal("tip")
	}
	if _, err := env.ancestor(context.Background(), "HEAD"); err == nil {
		t.Fatal("ancestor")
	}
	git(t, f.dir, "branch", "fb")
	if err := env.addWorktree(context.Background(), f.dir, "fb"); err == nil {
		t.Fatal("worktree")
	}
	env.Run = func(context.Context, string, string, string, ...string) ([]byte, error) {
		return nil, failure("ps down")
	}
	if _, err := env.caffeinePlan(context.Background(), false); err == nil {
		t.Fatal("caffeine")
	}
	f.hub.on(get("/issues/4"), Issue{Body: "ok"})
	if err := dispatchCmd(context.Background(), f.Env(t), []string{"4", "--model", "opus"}, ioDiscard()); err == nil {
		t.Fatal("forecast")
	}
	if err := dispatchCmd(context.Background(), f.Env(t), []string{"0", "--model", "opus"}, ioDiscard()); err == nil {
		t.Fatal("zero")
	}
	when := f.now
	f.hub.on(get("/issues/4"), Issue{Body: "Blocked by #8"})
	f.hub.on(get("/issues/8"), Issue{State: "open"})
	f.hub.on(
		list("/pulls?state=closed"),
		[]PR{{MergedAt: &when, Base: Ref{Ref: "fb"}, Body: "Closes #8", MergeCommitSHA: "missing"}},
	)
	if err := f.Env(t).blockersClear(context.Background(), 4); err == nil {
		t.Fatal("bad merge")
	}
	env = f.Env(t)
	future := f.now.Add(time.Hour).Unix()
	env.Run = gitStamp(future)
	f.hub.on(get("/issues/1"), Issue{})
	f.hub.on(list("/pulls?state=open"), []PR{{Body: "#1", UpdatedAt: f.now.Add(2 * time.Hour)}})
	if _, err := env.lastActivity(
		context.Background(),
		Record{Ticket: 1, Started: f.now, Worktree: f.dir},
	); err != nil {
		t.Fatal(err)
	}
	f.owner(t, Record{Ticket: 3, State: Running, Started: f.now, Worktree: f.dir})
	env = f.Env(t)
	env.Run = func(context.Context, string, string, string, ...string) ([]byte, error) {
		return nil, failure("git down")
	}
	if err := watchCmd(context.Background(), env, nil, ioDiscard()); err == nil {
		t.Fatal("watch activity")
	}
	env.Run = f.run
	writeFile(t, env.recordPath(40), "{")
	f.hub.on(get("/pulls/6"), headed(6, f.head(t)))
	git(t, f.dir, "update-ref", "refs/remotes/origin/fb", "fb")
	if err := conflictsCmd(context.Background(), env, []string{"6"}, ioDiscard()); err == nil {
		t.Fatal("record")
	}
	if err := setAgent(env, []string{"x", "a"}); err == nil {
		t.Fatal("own")
	}
	if err := setAgent(env, []string{"4"}); err == nil {
		t.Fatal("own usage")
	}
	if err := setAgent(env, []string{"5", "a"}); err == nil {
		t.Fatal("own missing")
	}
	h := newFixture(t)
	if code, _, _ := h.agents(t, "status", "--publish"); code != 1 {
		t.Fatal("status body")
	}
	if code, _, _ := h.agents(t, "dispatch", "4", "--model", "opus", "--dry-run"); code == 0 {
		t.Fatal("dry")
	}
	if err := h.Env(t).writeStatus(context.Background(), 1, true, "x"); err == nil {
		t.Fatal("patch")
	}
	env.Run = func(_ context.Context, _ string, _ string, name string, args ...string) ([]byte, error) {
		if name == "git" && len(args) > 1 && args[0] == "rev-parse" && args[1] == "--verify" {
			return []byte("ok\n"), nil
		}
		return nil, failure("merge down")
	}
	if _, _, err := env.mergeTree(context.Background(), headed(1, "abc")); err == nil {
		t.Fatal("merge")
	}
	env.Run = func(context.Context, string, string, string, ...string) ([]byte, error) {
		return []byte("10\n"), nil
	}
	if _, err := env.lastActivity(
		context.Background(),
		Record{Ticket: 99, Started: f.now, Worktree: f.dir},
	); err == nil {
		t.Fatal("issue")
	}
	f.hub.on(get("/issues/99"), Issue{})
	f.hub.on(list("/pulls?state=open"), "not-json")
	if _, err := env.lastActivity(
		context.Background(),
		Record{Ticket: 99, Started: f.now, Worktree: f.dir},
	); err == nil {
		t.Fatal("prs")
	}
	env.Run = gitStamp(f.now.Add(24 * time.Hour).Unix())
	f.hub.on(list("/pulls?state=open"), []PR{})
	if _, err := env.lastActivity(
		context.Background(),
		Record{Ticket: 99, Started: time.Unix(1, 0), Worktree: f.dir},
	); err != nil {
		t.Fatal(err)
	}
	env.Run = func(_ context.Context, _ string, _ string, name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "rev-parse" {
			return nil, failure("rev")
		}
		return []byte("10\n"), nil
	}
	if _, _, err := env.pushTime(context.Background(), f.dir); err == nil {
		t.Fatal("push")
	}
	f.owner(t, Record{Ticket: 8, State: Done, Worktree: f.dir})
	_ = os.Remove(f.Env(t).recordPath(40))
	env = f.Env(t)
	env.Run = func(_ context.Context, _ string, _ string, name string, args ...string) ([]byte, error) {
		if name == "lsof" {
			return nil, failure("lsof")
		}
		if name == "git" {
			return []byte("10\n"), nil
		}
		return []byte("1\n"), nil
	}
	f.hub.on(get("/issues/3"), Issue{})
	f.hub.on(list("/pulls?state=open"), []PR{})
	if err := watchCmd(context.Background(), env, nil, ioDiscard()); err == nil {
		t.Fatal("alive")
	}
	tip := f.Env(t)
	tip.Run = func(_ context.Context, _ string, _ string, _ string, args ...string) ([]byte, error) {
		if len(args) > 1 && args[1] == "--verify" {
			return nil, nil
		}
		return nil, failure("rev down")
	}
	if _, err := tip.featureTip(context.Background()); err == nil {
		t.Fatal("tip rev")
	}
	_ = os.Remove(f.Env(t).recordPath(40))
	for _, n := range []int{3, 8} {
		rec, err := f.Env(t).record(n)
		if err != nil {
			t.Fatal(err)
		}
		rec.State = Exited
		if err := f.Env(t).saveRecord(rec); err != nil {
			t.Fatal(err)
		}
	}
	d := f.Env(t)
	d.Run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "ps" {
			return nil, failure("ps down")
		}
		return f.run(ctx, dir, stdin, name, args...)
	}
	f.hub.on(get("/issues/4"), Issue{Body: "ready"})
	f.hub.on(list("/pulls?state=open"), []PR{})
	miss := newFixture(t)
	git(t, miss.dir, "commit", "-q", "--allow-empty", "-m", "root")
	miss.hub.on(get("/issues/4"), Issue{Body: "ready"})
	miss.hub.on(list("/pulls?state=open"), []PR{})
	if err := dispatchCmd(
		context.Background(),
		miss.Env(t),
		[]string{"4", "--model", "opus"},
		ioDiscard(),
	); err == nil ||
		!strings.Contains(err.Error(), "fb") {
		t.Fatal(err)
	}
	if err := dispatchCmd(
		context.Background(),
		d,
		[]string{"4", "--model", "opus"},
		ioDiscard(),
	); err == nil ||
		!strings.Contains(err.Error(), "ps down") {
		t.Fatal(err)
	}
	d.Run = f.run
	d.Start = func(string, ...string) error { return nil }
	d.Run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "ps" {
			return []byte("1 claude\n"), nil
		}
		if name == "pgrep" {
			return nil, failure("none")
		}
		if name == "git" && len(args) > 0 && args[0] == "worktree" {
			return nil, failure("worktree down")
		}
		return f.run(ctx, dir, stdin, name, args...)
	}
	if err := dispatchCmd(
		context.Background(),
		d,
		[]string{"4", "--model", "opus"},
		ioDiscard(),
	); err == nil ||
		!strings.Contains(err.Error(), "worktree down") {
		t.Fatal(err)
	}
	s := newFixture(t)
	s.hub.on(list("/pulls?state=open"), []PR{})
	s.hub.on(list("/issues/7/comments?"), []Comment{})
	if code, _, _ := s.agents(t, "status", "--publish"); code != 1 {
		t.Fatal("post")
	}
	c := prepBranch(t)
	git(t, c.dir, "update-ref", "refs/remotes/origin/fb", "fb")
	c.hub.on(get("/pulls/5"), headed(5, "deadbeef"))
	if code, _, _ := c.agents(t, "conflicts", "5"); code != 1 {
		t.Fatal("head")
	}
}

func gitStamp(future int64) Runner {
	return func(_ context.Context, _ string, _ string, name string, args ...string) ([]byte, error) {
		if name != "git" {
			return nil, failure("down")
		}
		if len(args) > 0 && args[0] == "rev-parse" && len(args) > 2 && args[1] == "--abbrev-ref" {
			return []byte("topic\n"), nil
		}
		if len(args) > 3 {
			return []byte(fmt.Sprintf("%d\n", future+10)), nil
		}
		if len(args) > 0 && args[0] == "log" {
			return []byte(fmt.Sprintf("%d\n", future)), nil
		}
		return []byte("topic\n"), nil
	}
}
