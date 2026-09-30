package agents

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
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
