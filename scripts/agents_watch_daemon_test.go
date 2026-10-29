package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const fakeWatch = `package main

import (
	"os"
	"os/signal"
	"syscall"
)

func main() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM)
	<-stop
}
`

func watchRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	writeFile(t, filepath.Join(repo, "apps/backend/go.mod"), "module example.com/backend\n\ngo 1.23\n")
	writeFile(t, filepath.Join(repo, "apps/backend/cmd/monacoctl/main.go"), fakeWatch)
	commitAll(t, repo, "feat: first watch")
	return repo
}

func commitAll(t *testing.T, repo, subject string) {
	t.Helper()
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.email=a@example.com", "-c", "user.name=a", "-c", "commit.gpgsign=false",
		"commit", "-q", "-m", subject)
}

func startWatchDaemon(t *testing.T, repo string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("bash", filepath.Join(repoRoot(t), "scripts/agents-watch-daemon.sh"), "run")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "MONACO_WATCH_STALE_POLL=0.2", "MONACO_WATCH_RESTART_AFTER=0", "GOTOOLCHAIN=local")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
	})
	return cmd
}

func daemonLog(repo string) string {
	b, _ := os.ReadFile(filepath.Join(repo, ".git/pstack/watch/logs/watch-daemon.log"))
	return string(b)
}

func TestWatchDaemon_rebuildsAndRestartsTheWatchWhenMonacoctlChanges(t *testing.T) {
	t.Parallel()
	repo := watchRepo(t)
	startWatchDaemon(t, repo)
	waitFor(t, "the first build and start", func() bool {
		return strings.Count(daemonLog(repo), "daemon: starting") == 1
	})
	if log := daemonLog(repo); !strings.Contains(log, "daemon: built ") {
		t.Fatalf("no build before the first start:\n%s", log)
	}

	writeFile(t, filepath.Join(repo, "apps/backend/cmd/monacoctl/main.go"), fakeWatch+"\nvar _ = 1\n")
	commitAll(t, repo, "fix: change the watch")
	waitFor(t, "a restart on the new build", func() bool {
		return strings.Count(daemonLog(repo), "daemon: starting") == 2
	})
	log := daemonLog(repo)
	if strings.Count(log, "daemon: built ") != 2 || !strings.Contains(log, "changed; restarting the watch on a new build") {
		t.Fatalf("restarted without a rebuild:\n%s", log)
	}
}

func TestWatchDaemon_keepsTheWatchRunningWhenOtherCodeChanges(t *testing.T) {
	t.Parallel()
	repo := watchRepo(t)
	startWatchDaemon(t, repo)
	waitFor(t, "the first start", func() bool {
		return strings.Count(daemonLog(repo), "daemon: starting") == 1
	})

	writeFile(t, filepath.Join(repo, "apps/backend/internal/a.go"), "package internal\n")
	commitAll(t, repo, "feat: other code")
	writeFile(t, filepath.Join(repo, "apps/backend/cmd/monacoctl/broken.go"), "package main\n\nfunc broken( {\n")
	commitAll(t, repo, "fix: a broken watch")
	waitFor(t, "the failed build", func() bool {
		return strings.Contains(daemonLog(repo), "failed; keeping the old binary")
	})
	writeFile(t, filepath.Join(repo, "apps/backend/README.md"), "x\n")
	commitAll(t, repo, "docs: other code again")
	waitFor(t, "a start on the old binary", func() bool {
		return strings.Count(daemonLog(repo), "daemon: starting") == 2
	})
	if log := daemonLog(repo); strings.Count(log, "restarting the watch on a new build") != 1 ||
		strings.Count(log, "daemon: built ") != 1 {
		t.Fatalf("restarted on a change outside cmd/monacoctl or after the failed build:\n%s", log)
	}
}
