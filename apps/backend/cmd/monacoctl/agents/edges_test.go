package agents

import (
	"context"
	"os"
	"strings"
	"testing"
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
		[]PR{{MergedAt: &when, Base: Ref{Ref: "fb"}, Body: "Closes #8", MergeCommitSHA: side}},
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

func TestEdges_uncoveredDispatchAndConflicts(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	env := f.Env(t)
	if code, _, _ := f.agents(t, "conflicts"); code != 2 {
		t.Fatal("usage")
	}
	if code, _, _ := f.agents(t, "conflicts", "3"); code != 1 {
		t.Fatal("pr")
	}
	git(t, f.dir, "update-ref", "refs/remotes/origin/fb", "fb")
	f.hub.on(get("/pulls/3"), headed(3, "dead"))
	if code, _, _ := f.agents(t, "conflicts", "3"); code != 1 {
		t.Fatal("sha")
	}
	writeFile(t, env.recordPath(40), "{")
	f.hub.on(get("/pulls/3"), headed(3, f.head(t)))
	if err := conflictsCmd(context.Background(), env, []string{"3"}, ioDiscard()); err == nil {
		t.Fatal("bad record")
	}
	_ = os.Remove(env.recordPath(40))
	var buf strings.Builder
	files := make([]string, 20)
	if err := writeRebase(
		&buf,
		pr(1, "h", "fb", ""),
		Record{},
		false,
		files,
	); err != nil ||
		!strings.Contains(buf.String(), "and ") {
		t.Fatal(buf.String(), err)
	}
	if err := setAgent(env, nil); err == nil {
		t.Fatal("own usage")
	}
	if err := setAgent(env, []string{"x", "a"}); err == nil {
		t.Fatal("own int")
	}
	env.Run = func(_ context.Context, _ string, _ string, _ string, args ...string) ([]byte, error) {
		if len(args) > 1 && args[1] == "--verify" {
			return nil, nil
		}
		return nil, failure("merge down")
	}
	if _, _, err := env.mergeTree(context.Background(), headed(1, "abc")); err == nil {
		t.Fatal("merge")
	}
	miss := newFixture(t)
	git(t, miss.dir, "commit", "-q", "--allow-empty", "-m", "root")
	miss.hub.on(get("/issues/4"), Issue{Body: "ready"})
	miss.hub.on(list("/pulls?state=open"), []PR{})
	if err := dispatchCmd(
		context.Background(),
		miss.Env(t),
		[]string{"4", "--model", "opus"},
		ioDiscard(),
	); err == nil {
		t.Fatal("tip")
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
		t.Fatal("merge sha")
	}
	if closes("nope", 1) || !closes("Closes #1", 1) {
		t.Fatal("closes")
	}
	bare := newFixture(t)
	if _, err := bare.Env(t).ancestor(context.Background(), "HEAD"); err == nil {
		t.Fatal("no ref")
	}
	tip := f.Env(t)
	tip.Run = func(_ context.Context, _ string, _ string, _ string, args ...string) ([]byte, error) {
		if len(args) > 1 && args[1] == "--verify" {
			return nil, nil
		}
		return nil, failure("rev down")
	}
	if _, err := tip.featureTip(context.Background()); err == nil {
		t.Fatal("rev")
	}
	d := f.Env(t)
	d.Start = func(string, ...string) error { return nil }
	d.Run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "ps" {
			return nil, failure("ps down")
		}
		return Exec(ctx, dir, stdin, name, args...)
	}
	f.hub.on(get("/issues/4"), Issue{Body: "ready"})
	f.hub.on(list("/pulls?state=open"), []PR{})
	if err := dispatchCmd(
		context.Background(),
		d,
		[]string{"4", "--model", "opus"},
		ioDiscard(),
	); err == nil ||
		!strings.Contains(err.Error(), "ps down") {
		t.Fatal(err)
	}
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
		return Exec(ctx, dir, stdin, name, args...)
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
	if err := setAgent(f.Env(t), []string{"5", "a"}); err == nil {
		t.Fatal("missing owner")
	}
	plain := newFixture(t)
	git(t, plain.dir, "commit", "-q", "--allow-empty", "-m", "root")
	git(t, plain.dir, "branch", "fb")
	plain.hub.on(get("/issues/4"), Issue{Body: "ready"})
	if err := dispatchCmd(
		context.Background(),
		plain.Env(t),
		[]string{"4", "--model", "opus"},
		ioDiscard(),
	); err == nil {
		t.Fatal("forecast")
	}
	plain.hub.on(get("/pulls/1"), headed(1, "abc"))
	if code, _, stderr := plain.agents(t, "conflicts", "1"); code != 1 || !strings.Contains(stderr, "origin/fb") {
		t.Fatalf("no origin %d %q", code, stderr)
	}
}
