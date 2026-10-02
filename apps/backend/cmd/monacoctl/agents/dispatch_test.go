package agents

import (
	"context"
	"fmt"
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
