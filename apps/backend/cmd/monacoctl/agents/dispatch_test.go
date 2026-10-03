package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	psCaller  = "/tmp/monacoctl"
	psShell   = "/bin/zsh"
	psCLI     = "/usr/local/bin/claude"
	psLaunchd = "/sbin/launchd"
	psHook    = "/Users/dev/.claude/hooks/notify.sh"
	psDesktop = "/Users/dev/Library/Application Support/Claude/claude-code/2.1.284/claude.app/Contents/MacOS/claude"
)

func psChain(commands ...string) Runner {
	self := os.Getpid()
	return func(_ context.Context, _, _, name string, args ...string) ([]byte, error) {
		if name != "ps" || len(args) == 0 {
			return nil, fmt.Errorf("unexpected command %s %v", name, args)
		}
		pid, err := strconv.Atoi(args[len(args)-1])
		at := pid - self
		if err != nil || at < 0 || at >= len(commands) {
			return nil, fmt.Errorf("ps: no process %s", args[len(args)-1])
		}
		parent := pid + 1
		if at == len(commands)-1 {
			parent = 1
		}
		return fmt.Appendf(nil, "%8d %s\n", parent, commands[at]), nil
	}
}

func TestDispatch_findsTheClaudeProcessAboveTheCaller(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		commands []string
		above    int
	}{
		{"path with a space", []string{psCaller, psShell, psDesktop}, 2},
		{"path without a space", []string{psCaller, psShell, psCLI}, 2},
		{"claude only in a directory name", []string{psHook, psShell, psCLI}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := &Env{Run: psChain(tc.commands...)}
			want := os.Getpid() + tc.above
			if got, err := env.claudePID(t.Context()); err != nil || got != want {
				t.Fatalf("got pid %d, error %v; want pid %d", got, err, want)
			}
		})
	}
}

func TestDispatch_namesThePidItSearchedFromWhenNoClaudeIsAbove(t *testing.T) {
	t.Parallel()
	env := &Env{Run: psChain(psCaller, psShell, psLaunchd)}
	_, err := env.claudePID(t.Context())
	if err == nil {
		t.Fatal("want an error")
	}
	want := fmt.Sprintf("no claude process above pid %d", os.Getpid())
	if got := cliText(err); !strings.Contains(got, want) {
		t.Fatalf("got %q; want text containing %q", got, want)
	}
}

func TestCaffeineOnPath_followsLookPath(t *testing.T) {
	t.Parallel()
	_, wantErr := exec.LookPath("caffeinate")
	if (&Env{}).caffeineOnPath() != (wantErr == nil) {
		t.Fatal("nil LookPath did not follow exec.LookPath")
	}
	if (&Env{LookPath: absentCaffeinate}).caffeineOnPath() {
		t.Fatal("a missing binary reported present")
	}
	if !(&Env{LookPath: foundCaffeinate}).caffeineOnPath() {
		t.Fatal("a present binary reported missing")
	}
}

func TestDispatch_dryRunWithoutCaffeinatePrintsTheNote(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.lookPath = absentCaffeinate
	f.batch(t, 12)
	f.hub.on(get("/issues/12"), Issue{Number: 12, Body: "**Milestone:** M7 · **Blocked by:** none · **Touches:** `a`"})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.ps()
	code, stdout, stderr := f.agents(t, "dispatch", "12", "--model", "opus", "--dry-run")
	note := "caffeinate not found; the host must stay awake on its own"
	if code != 0 || stderr != "" || !strings.Contains(stdout, note) ||
		strings.Contains(stdout, "would start caffeinate") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(f.Env(t).recordPath(12)); !os.IsNotExist(err) {
		t.Fatalf("record written: %v", err)
	}
}

func TestDispatch_withoutCaffeinateStillCreatesTheWorktree(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.batch(t, 12)
	f.hub.on(get("/issues/12"), Issue{Body: "**Milestone:** M7 · **Blocked by:** none · **Touches:** `a`"})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.ownerComments(12)
	f.ps()
	started := false
	env := f.Env(t)
	env.LookPath = absentCaffeinate
	env.Start = func(string, ...string) error {
		started = true
		return nil
	}
	env.Run = f.run
	if err := dispatchCmd(context.Background(), env, []string{"12", "--model", "opus"}, ioDiscard()); err != nil {
		t.Fatal(err)
	}
	if started {
		t.Fatal("started caffeinate")
	}
	rec, err := env.localRecord(12)
	if err != nil || rec.State != Running {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
	if _, err := os.Stat(rec.Worktree); err != nil {
		t.Fatal(err)
	}
}

func TestWatch_skipsTheCaffeinateLineWhenTheBinaryIsMissing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.lookPath = absentCaffeinate
	old := f.now.Add(-time.Hour)
	f.owner(t, Record{Ticket: 1, State: Running, Started: old, Worktree: f.dir})
	f.hub.on(get("/issues/1"), Issue{UpdatedAt: old})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.watchGit("HEAD", "1", "")
	f.noFailures()
	code, stdout, stderr := f.agents(t, "watch", "--once")
	if code != 1 || stderr != "" || !strings.Contains(stdout, "idle: #1") ||
		strings.Contains(stdout, "missing caffeinate") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func numberedDraft(n int, at time.Time, jobs ...string) string {
	return fmt.Sprintf(`{"number":%d,"state":"CLOSED","title":"Merge queue","body":"","headRefName":"gtmq_%d",`+
		`"updatedAt":%q,"commits":{"nodes":[{"commit":%s}]}}`, n, n, at.Format(time.RFC3339), rollup(jobs...))
}

func TestDispatch_refusesAboveMaxLoadAndUrgentDispatches(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.batch(t, 12)
	f.load = 20
	f.hub.on(get("/issues/12"), Issue{Body: "no blockers"})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.ownerComments(12)
	f.hub.on("POST /repos/o/r/issues/7/comments", "{}")
	f.ps()
	code, _, stderr := f.agents(t, "dispatch", "12", "--model", "opus")
	if code != 1 || !strings.Contains(stderr, "load1 20.0 is over max_load 12; wait or dispatch with --urgent") {
		t.Fatalf("refuse: %d %q", code, stderr)
	}
	if _, err := os.Stat(f.Env(t).recordPath(12)); !os.IsNotExist(err) {
		t.Fatalf("record written: %v", err)
	}
	code, _, stderr = f.agents(t, "dispatch", "12", "--model", "opus", "--urgent")
	if code != 0 {
		t.Fatalf("urgent: %d %q", code, stderr)
	}
	if rec, err := f.Env(t).localRecord(12); err != nil || rec.State != Running {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
}

func TestQueueGate_refusesTwoRecentDraftsFailingOnOneJob(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	recent, old := f.now.Add(-10*time.Minute), f.now.Add(-61*time.Minute)
	open := strings.Replace(numberedDraft(5, recent, flakeJob), `"CLOSED"`, `"OPEN"`, 1)
	notQueue := strings.Replace(numberedDraft(6, recent, flakeJob), "gtmq_6", "feature", 1)
	bare := `{"number":7,"state":"CLOSED","title":"","body":"","headRefName":"gtmq_7","updatedAt":` +
		strconv.Quote(recent.Format(time.RFC3339)) + `,"commits":{"nodes":[]}}`
	cases := []struct {
		name   string
		drafts []string
		want   string
	}{
		{
			"same job twice",
			[]string{numberedDraft(9, recent, flakeJob), numberedDraft(3, recent, flakeJob)},
			"queue is failing on ci / Flake (drafts #3, #9); fix the pipeline first, or dispatch the fix with --urgent",
		},
		{"later job repeats", []string{
			numberedDraft(1, recent, lintJob), numberedDraft(2, recent, flakeJob),
			numberedDraft(3, recent, flakeJob),
		}, "queue is failing on ci / Flake (drafts #2, #3)"},
		{"different jobs", []string{numberedDraft(1, recent, flakeJob), numberedDraft(2, recent, lintJob)}, ""},
		{
			"one older than 60 minutes",
			[]string{numberedDraft(1, recent, flakeJob), numberedDraft(2, old, flakeJob)},
			"",
		},
		{"passing, open, non-queue and empty drafts", []string{
			numberedDraft(1, recent, okJob), numberedDraft(2, recent, flakeJob), open, notQueue, bare,
		}, ""},
	}
	for _, tc := range cases {
		f.hub.on(graphqlRoute, draftData(tc.drafts))
		ok, err := f.Env(t).queueGate(t.Context())
		switch {
		case tc.want == "" && (err != nil || ok == ""):
			t.Errorf("%s: got %q, %v; want pass", tc.name, ok, err)
		case tc.want != "" && (err == nil || !strings.Contains(cliText(err), tc.want)):
			t.Errorf("%s: got %q, %v; want %q", tc.name, ok, err, tc.want)
		}
	}
	f.hub.on(graphqlRoute, `{"data":null,"errors":[{"message":"rate limited"}]}`)
	if _, err := f.Env(t).queueGate(t.Context()); cliText(err) != "graphql: rate limited" {
		t.Fatalf("graphql: %v", err)
	}
}

func TestDispatch_dryRunPrintsBothGateResultsAndChangesNothing(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.batch(t, 12)
	f.hub.on(get("/issues/12"), Issue{Body: "no blockers"})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.ownerComments(12)
	f.ps()
	recent := f.now.Add(-time.Minute)
	f.load = 3
	code, stdout, stderr := f.agents(t, "dispatch", "12", "--model", "opus", "--dry-run")
	if code != 0 || stderr != "" ||
		!strings.Contains(stdout, "dry-run: load gate would pass: load1 3.0, max_load 12\n") ||
		!strings.Contains(stdout, "dry-run: queue gate would pass: no job failed in two queue drafts "+
			"in the last 60 minutes\n") {
		t.Fatalf("pass: %d %q %q", code, stdout, stderr)
	}
	f.load = 20
	f.hub.on(graphqlRoute, draftData([]string{numberedDraft(3, recent, flakeJob), numberedDraft(4, recent, flakeJob)}))
	code, stdout, stderr = f.agents(t, "dispatch", "12", "--model", "opus", "--dry-run")
	if code != 0 || stderr != "" ||
		!strings.Contains(
			stdout,
			"dry-run: load gate would refuse: load1 20.0 is over max_load 12; wait or dispatch with --urgent\n",
		) ||
		!strings.Contains(stdout, "dry-run: queue gate would refuse: queue is failing on ci / Flake (drafts #3, #4); "+
			"fix the pipeline first, or dispatch the fix with --urgent\n") {
		t.Fatalf("refuse: %d %q %q", code, stdout, stderr)
	}
	code, stdout, _ = f.agents(t, "dispatch", "12", "--model", "opus", "--dry-run", "--urgent")
	if code != 0 || !strings.Contains(stdout, "dry-run: load gate and queue breaker bypassed by --urgent\n") {
		t.Fatalf("urgent: %d %q", code, stdout)
	}
	if _, err := os.Stat(f.Env(t).recordPath(12)); !os.IsNotExist(err) {
		t.Fatalf("record written: %v", err)
	}
	if len(f.hub.callsContaining("POST /repos")) != 0 {
		t.Fatalf("posted %v", f.hub.callsContaining("POST /repos"))
	}
}

func TestGates_surfaceALoadReadFailure(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.hub.on(graphqlRoute, failureData())
	env := f.Env(t)
	env.Load = func(context.Context, string) (float64, error) { return 0, errors.New("no load") }
	if err := env.gates(
		t.Context(),
		dispatchIn{},
		io.Discard,
	); err == nil ||
		!strings.Contains(err.Error(), "no load") {
		t.Fatalf("got %v", err)
	}
	var out strings.Builder
	if err := env.gates(t.Context(), dispatchIn{dry: true}, &out); err != nil ||
		!strings.Contains(out.String(), "dry-run: load gate would refuse: ") {
		t.Fatalf("dry: %v %q", err, out.String())
	}
	env.Load, env.Config.MaxLoad = nil, 1000000
	if _, err := env.loadGate(t.Context()); err != nil {
		t.Fatalf("host load: %v", err)
	}
}
