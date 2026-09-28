package scripts_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func startShell(t *testing.T, command string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sh", "-c", "eval "+shellQuote(command))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	return cmd
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func exited(cmd *exec.Cmd, within time.Duration) bool {
	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(within):
		return false
	}
}

func TestAgentReap_stopsOnlyTheFinishedAgentsShellsAndDropsDeadQueueTickets(t *testing.T) {
	dir := t.TempDir()
	mine := startShell(t, "sleep 301 && echo 'done'")
	theirs := startShell(t, "sleep 302")
	transcript := filepath.Join(dir, "agent.jsonl")
	if err := os.WriteFile(transcript, []byte(`{"content":"Command running in background with ID: bmine0001"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	queue := filepath.Join(dir, "m7-heavy.queue")
	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	deadTicket := filepath.Join(queue, fmt.Sprintf("100-%d", dead.Process.Pid))
	liveTicket := filepath.Join(queue, fmt.Sprintf("200-%d", os.Getpid()))
	for _, p := range []string{deadTicket, liveTicket} {
		writeExecutable(t, p, "go test ./...\n")
	}

	r := runHook(t, "agent-reap.py", map[string]any{
		"hook_event_name":       "SubagentStop",
		"agent_transcript_path": transcript,
		"background_tasks": []map[string]any{
			{"id": "asub", "type": "subagent", "status": "running"},
			{"id": "bmine0001", "type": "shell", "status": "running", "command": "sleep 301 && echo 'done'"},
			{"id": "btheirs01", "type": "shell", "status": "running", "command": "sleep 302"},
		},
	}, fmt.Sprintf("CLAUDE_PID=%d", os.Getpid()), "HEAVY_QUEUE_GLOB="+filepath.Join(dir, "*heavy.queue"))

	if r.code != 0 {
		t.Fatalf("hook exit %d: %s", r.code, r.stderr)
	}
	if !exited(mine, 5*time.Second) {
		t.Error("the finished agent's own background shell is still running")
	}
	if exited(theirs, 500*time.Millisecond) {
		t.Error("another agent's background shell was killed")
	}
	if _, err := os.Stat(deadTicket); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stale queue ticket kept: %v", err)
	}
	if _, err := os.Stat(liveTicket); err != nil {
		t.Errorf("live queue ticket removed: %v", err)
	}
	if !strings.Contains(r.stdout, "stopped background shell bmine0001") {
		t.Errorf("want a report of the stopped shell, got %q", r.stdout)
	}
}
