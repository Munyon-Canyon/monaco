package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

func (f *fixture) stageZeroTickets(t *testing.T, live int) {
	t.Helper()
	dir := filepath.Join(f.Env(t).Common, ".monaco", "check-queue")
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "0-dead"), "/work\n")
	for i := 1; i <= live; i++ {
		writeFile(t, filepath.Join(dir, fmt.Sprintf("%d-%d", i, os.Getpid())), "/work\n")
	}
	writeFile(t, filepath.Join(f.dir, ".git", localConfigPath), "[check]\nslots = 4\n")
}

func (f *fixture) dispatchableTicket(t *testing.T) {
	t.Helper()
	f.batch(t, 12)
	f.hub.on(get("/issues/12"), Issue{Body: "no blockers"})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.ownerComments(12)
	f.hub.on("POST /repos/o/r/issues/7/comments", "{}")
	f.ps()
}

func TestDispatch_refusesAboveTheBacklogLimitAndUrgentDispatches(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.dispatchableTicket(t)
	f.stageZeroTickets(t, 13)
	code, _, stderr := f.agents(t, "dispatch", "12", "--model", "opus")
	if code != 1 || !strings.Contains(stderr, "stage 0 queue holds 13 live tickets, over the limit of 12 "+
		"(max_queue 3 x 4 slots); wait or dispatch with --urgent") {
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

func TestDispatch_proceedsAtTheBacklogLimit(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.dispatchableTicket(t)
	f.stageZeroTickets(t, 12)
	code, _, stderr := f.agents(t, "dispatch", "12", "--model", "opus")
	if code != 0 {
		t.Fatalf("dispatch: %d %q", code, stderr)
	}
	if rec, err := f.Env(t).localRecord(12); err != nil || rec.State != Running {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
}

func TestBacklogGate_countsOnlyLiveTicketsAndPassesWithoutAQueueDirectory(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	if msg, err := env.backlogGate(t.Context()); err != nil || !strings.HasPrefix(msg, "0 live tickets") {
		t.Fatalf("no directory: %q %v", msg, err)
	}
	f.stageZeroTickets(t, 6)
	env.Config.Slots, env.Config.MaxQueue = 2, 3
	if msg, err := env.backlogGate(t.Context()); err != nil || msg != "6 live tickets, limit 6" {
		t.Fatalf("dead ticket counted: %q %v", msg, err)
	}
	env.Config.MaxQueue = 2
	_, err := env.backlogGate(t.Context())
	if err == nil || !strings.Contains(cliText(err), "6 live tickets, over the limit of 4") {
		t.Fatalf("limit not scaled by max_queue: %v", err)
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
		{
			"push-only workflow failing on the draft head",
			[]string{numberedDraft(1, recent, goCacheJob), numberedDraft(2, recent, goCacheJob)},
			"",
		},
		{
			"queue CI job behind a push-only failure",
			[]string{numberedDraft(1, recent, goCacheJob, readyJob), numberedDraft(2, recent, readyJob)},
			"queue is failing on ci / Ready (staging) (drafts #1, #2)",
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
	f.stageZeroTickets(t, 2)
	code, stdout, stderr := f.agents(t, "dispatch", "12", "--model", "opus", "--dry-run")
	if code != 0 || stderr != "" ||
		!strings.Contains(stdout, "dry-run: backlog gate would pass: 2 live tickets, limit 12\n") ||
		!strings.Contains(stdout, "dry-run: queue gate would pass: no job failed in two queue drafts "+
			"in the last 60 minutes\n") {
		t.Fatalf("pass: %d %q %q", code, stdout, stderr)
	}
	f.stageZeroTickets(t, 13)
	f.hub.on(graphqlRoute, draftData([]string{numberedDraft(3, recent, flakeJob), numberedDraft(4, recent, flakeJob)}))
	code, stdout, stderr = f.agents(t, "dispatch", "12", "--model", "opus", "--dry-run")
	if code != 0 || stderr != "" ||
		!strings.Contains(
			stdout,
			"dry-run: backlog gate would refuse: stage 0 queue holds 13 live tickets, over the limit of 12 "+
				"(max_queue 3 x 4 slots); wait or dispatch with --urgent\n",
		) ||
		!strings.Contains(stdout, "dry-run: queue gate would refuse: queue is failing on ci / Flake (drafts #3, #4); "+
			"fix the pipeline first, or dispatch the fix with --urgent\n") {
		t.Fatalf("refuse: %d %q %q", code, stdout, stderr)
	}
	code, stdout, _ = f.agents(t, "dispatch", "12", "--model", "opus", "--dry-run", "--urgent")
	if code != 0 || !strings.Contains(stdout, "dry-run: backlog gate and queue breaker bypassed by --urgent\n") {
		t.Fatalf("urgent: %d %q", code, stdout)
	}
	if _, err := os.Stat(f.Env(t).recordPath(12)); !os.IsNotExist(err) {
		t.Fatalf("record written: %v", err)
	}
	if len(f.hub.callsContaining("POST /repos")) != 0 {
		t.Fatalf("posted %v", f.hub.callsContaining("POST /repos"))
	}
}

func TestGates_surfaceAQueueReadFailure(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.hub.on(graphqlRoute, failureData())
	env := f.Env(t)
	writeFile(t, filepath.Join(env.Common, ".monaco", "check-queue"), "not a directory")
	if err := env.gates(t.Context(), dispatchIn{}, io.Discard); err == nil ||
		!strings.Contains(err.Error(), "read the stage 0 queue") {
		t.Fatalf("got %v", err)
	}
	var out strings.Builder
	if err := env.gates(t.Context(), dispatchIn{dry: true}, &out); err != nil ||
		!strings.Contains(out.String(), "dry-run: backlog gate would refuse: ") {
		t.Fatalf("dry: %v %q", err, out.String())
	}
}

func TestDispatch_dryRunNamesTheLocalConfigOnlyWhenItExists(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.batch(t, 12)
	f.hub.on(get("/issues/12"), Issue{Number: 12, Body: "**Milestone:** M7 · **Blocked by:** none · **Touches:** `a`"})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.ps()
	code, stdout, stderr := f.agents(t, "dispatch", "12", "--model", "opus", "--dry-run")
	if code != 0 || strings.Contains(stdout, "local config:") {
		t.Fatalf("without a local file: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	local := filepath.Join(f.dir, ".git", localConfigPath)
	writeFile(t, local, "lanes = 6\ntracking = 512\n[check]\nslots = 4\n")
	code, stdout, stderr = f.agents(t, "dispatch", "12", "--model", "opus", "--dry-run")
	want := filepath.Join(".git", localConfigPath) +
		" (lanes=6, check.slots=4, dispatch.max_queue=3, tracking=512, milestone=ms)\n"
	if code != 0 || strings.Count(stdout, "local config: ") != 1 || !strings.Contains(stdout, want) {
		t.Fatalf("with a local file: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func (f *fixture) allowDispatch(t *testing.T) {
	t.Helper()
	f.batch(t, 12)
	f.hub.on(get("/issues/12"), Issue{Body: "no blockers"})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.ownerComments(12)
	f.ps()
}

func TestDispatch_linksThePinnedToolsIntoTheWorktree(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.allowDispatch(t)
	if code, _, stderr := f.agents(t, "dispatch", "12", "--model", "opus"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	env := f.Env(t)
	rec, err := env.localRecord(12)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range pinnedTools() {
		src, dst := env.cloneTool(tool), filepath.Join(rec.Worktree, ".bin", tool)
		want, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(dst)
		if err != nil || string(got) != string(want) || string(want) != "pinned "+tool+"\n" {
			t.Fatalf("%s: worktree has %q, clone has %q, err %v", tool, got, want, err)
		}
		srcInfo, _ := os.Stat(src)
		dstInfo, _ := os.Stat(dst)
		if !os.SameFile(srcInfo, dstInfo) {
			t.Fatalf("%s is a copy, want a hard link to the clone's", tool)
		}
	}
}

func TestDispatch_refusesBeforeAddingAWorktreeWhenTheCloneLacksAPinnedTool(t *testing.T) {
	t.Parallel()
	for _, tool := range pinnedTools() {
		t.Run(tool, func(t *testing.T) {
			t.Parallel()
			f := prepBranch(t)
			f.allowDispatch(t)
			env := f.Env(t)
			missing := env.cloneTool(tool)
			if err := os.Remove(missing); err != nil {
				t.Fatal(err)
			}
			want := "dispatch: " + missing + " is missing; run just install"
			for _, args := range [][]string{
				{"dispatch", "12", "--model", "opus"},
				{"dispatch", "12", "--model", "opus", "--dry-run"},
			} {
				code, stdout, stderr := f.agents(t, args...)
				if code != 1 || !strings.Contains(stderr, want) || strings.Contains(stdout, "spawn:") {
					t.Fatalf("%v: code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
				}
			}
			if _, err := os.Stat(env.worktreePath(12)); !os.IsNotExist(err) {
				t.Fatalf("worktree left behind: %v", err)
			}
			if _, err := os.Stat(env.recordPath(12)); !os.IsNotExist(err) {
				t.Fatalf("record written: %v", err)
			}
			writeFile(t, missing, "pinned "+tool+"\n")
			if code, _, stderr := f.agents(t, "dispatch", "12", "--model", "opus"); code != 0 {
				t.Fatalf("retry after the install: code=%d stderr=%q", code, stderr)
			}
		})
	}
}

func TestAddWorktree_reportsAToolItCannotProvide(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	env := f.Env(t)
	src := env.cloneTool("atlas")
	wt := filepath.Join(t.TempDir(), "wt")
	env.Run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "git" && len(args) > 0 && args[0] == "worktree" {
			return nil, os.WriteFile(wt, []byte("a file where the worktree should be\n"), 0o600)
		}
		return f.run(ctx, dir, stdin, name, args...)
	}
	if err := env.addWorktree(t.Context(), wt, "fb"); err == nil ||
		!strings.Contains(err.Error(), "make "+filepath.Join(wt, ".bin")) {
		t.Fatalf("link: got %v", err)
	}
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if err := env.addWorktree(t.Context(), wt, "fb"); err == nil || !strings.Contains(err.Error(), "stat "+src) {
		t.Fatalf("stat: got %v", err)
	}
}

func TestLinkOrCopy_linksWhenItCanAndCopiesWhenItCannot(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		link func(oldname, newname string) error
		same bool
	}{
		{"hard link", os.Link, true},
		{"copy", func(string, string) error { return errors.New("cross-device link") }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			src, dst := filepath.Join(dir, "clone", "sqlc"), filepath.Join(dir, "worktree", ".bin", "sqlc")
			writeFile(t, src, "pinned\n")
			if err := os.Chmod(src, 0o400); err != nil {
				t.Fatal(err)
			}
			if err := linkOrCopy(tc.link, src, dst, 0o400); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(dst)
			if err != nil || string(got) != "pinned\n" {
				t.Fatalf("content %q, err %v", got, err)
			}
			srcInfo, _ := os.Stat(src)
			dstInfo, _ := os.Stat(dst)
			if os.SameFile(srcInfo, dstInfo) != tc.same || dstInfo.Mode().Perm() != 0o400 {
				t.Fatalf("same file %v, mode %v; want same file %v, mode 0400", os.SameFile(srcInfo, dstInfo),
					dstInfo.Mode().Perm(), tc.same)
			}
		})
	}
}

func TestLinkOrCopy_namesTheStepThatFailed(t *testing.T) {
	t.Parallel()
	noLink := func(string, string) error { return errors.New("cross-device link") }
	dir := t.TempDir()
	good, gone, plain := filepath.Join(dir, "good"), filepath.Join(dir, "gone"), filepath.Join(dir, "plain")
	writeFile(t, good, "pinned\n")
	writeFile(t, plain, "already here\n")
	tests := []struct{ name, src, dst, want string }{
		{"make", good, filepath.Join(plain, "sqlc"), "make " + plain},
		{"open", gone, filepath.Join(dir, "a", "sqlc"), "open " + gone},
		{"create", good, plain, "create " + plain},
		{"copy", dir, filepath.Join(dir, "b", "sqlc"), "copy " + dir},
	}
	for _, tc := range tests {
		if err := linkOrCopy(noLink, tc.src, tc.dst, 0o600); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want text %q", tc.name, err, tc.want)
		}
	}
	if got, err := os.ReadFile(plain); err != nil || string(got) != "already here\n" {
		t.Fatalf("a failed copy changed the existing file: %q %v", got, err)
	}
}

func TestPinnedTools_areTheTwoBinariesGoGenerateAndMigrateNeed(t *testing.T) {
	t.Parallel()
	if got := strings.Join(pinnedTools(), " "); got != "atlas sqlc" {
		t.Fatalf("got %q", got)
	}
}

func TestDispatch_dryRunPrintsTheWholeSpawnBlockPastSixtyRemoteOwners(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.lookPath = absentCaffeinate
	f.batch(t, 12)
	f.hub.on(get("/issues/12"), Issue{Number: 12, Body: "**Milestone:** M7 · **Blocked by:** none · **Touches:** `a`"})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.ps()
	for n := 1001; n <= 1060; n++ {
		f.owner(t, Record{Ticket: n, State: Running, Worktree: filepath.Join(t.TempDir(), "gone")})
	}
	code, stdout, stderr := f.agents(t, "dispatch", "12", "--model", "opus", "--dry-run")
	env := f.Env(t)
	tip, err := env.featureTip(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	spawn := fmt.Sprintf("spawn: Agent subagent_type=%s model=opus run_in_background=true, prompt:\n"+
		"ticket: 12\nworktree: %s\nparent: %s\nbrief: %s\norders: %s\n",
		agentType, env.worktreePath(12), tip, ownerBrief, standingOrders)
	summary := "not counted: 60 records with worktrees on another machine " +
		"(#1001 #1002 #1003 #1004 #1005 #1006 #1007 #1008 #1009 #1010 ...)\n"
	if code != 0 || stderr != "" || !strings.Contains(stdout, spawn) ||
		strings.Count(stdout, "not counted") != 1 || !strings.Contains(stdout, summary) {
		t.Fatalf("code=%d stderr=%q stdout=\n%s", code, stderr, stdout)
	}
}
