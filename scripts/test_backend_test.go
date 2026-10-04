package scripts_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTestBackend_exitsOneAndPrintsTheFailedTestsWhenGoTestFails(t *testing.T) {
	t.Parallel()
	stub := t.TempDir()
	goStub := "#!/bin/sh\necho '{\"Action\":\"fail\",\"Package\":\"p\",\"Test\":\"TestX\"}'\nexit 1\n"
	writeExecutable(t, filepath.Join(stub, "go"), goStub)
	cmd := exec.Command(filepath.Join(repoRoot(t), "scripts", "test-backend.sh"))
	cmd.Dir = gitRepo(t)
	cmd.Env = append(os.Environ(),
		"PATH="+stub+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MONACO_TEST_BACKEND_CACHE="+t.TempDir(),
		"TEST_BACKEND_CACHE_EXEC=",
		"TEST_BACKEND_FORCE=",
	)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("test-backend.sh returned %v, want exit 1:\n%s", err, out)
	}
	if !strings.Contains(string(out), "--- output of failed tests ---") {
		t.Fatalf("test-backend.sh printed no failed tests section:\n%s", out)
	}
}
