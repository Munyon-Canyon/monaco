package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runRequireDocker runs scripts/require-docker.sh with a fake docker first on PATH.
func runRequireDocker(t *testing.T, dockerBody string) (out string, code int, took time.Duration) {
	t.Helper()
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "docker"), "#!/bin/sh\n"+dockerBody+"\n")
	cmd := exec.Command("bash", filepath.Join(repoRoot(t), "scripts", "require-docker.sh"))
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "MONACO_DOCKER_INFO_TIMEOUT=2")
	start := time.Now()
	b, err := cmd.CombinedOutput()
	took = time.Since(start)
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(b), code, took
}

func TestRequireDockerHungDaemonFailsFast(t *testing.T) {
	out, code, took := runRequireDocker(t, "exec sleep 30")
	if code != 1 || took > 5*time.Second {
		t.Fatalf("exit %d after %v, want exit 1 within 5s\n%s", code, took, out)
	}
	if !strings.Contains(out, "error: docker did not answer within 2s; Docker Desktop is hung. Quit and reopen it (or: docker desktop restart), then retry.") {
		t.Fatalf("missing hung message:\n%s", out)
	}
	t.Logf("hung docker: exit %d in %v", code, took)
}

func TestRequireDockerStoppedDaemonKeepsNotRunningMessage(t *testing.T) {
	out, code, took := runRequireDocker(t, "exit 1")
	if code != 1 || took > 2*time.Second || !strings.Contains(out, "docker daemon is not running") {
		t.Fatalf("exit %d after %v:\n%s", code, took, out)
	}
}

func TestRequireDockerHealthyDaemonPasses(t *testing.T) {
	out, code, _ := runRequireDocker(t, "exit 0")
	if code != 0 || out != "" {
		t.Fatalf("exit %d, output %q, want silent success", code, out)
	}
}
