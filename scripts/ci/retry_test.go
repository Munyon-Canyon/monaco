package ci_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func runRetry(t *testing.T, attempts int, failures int, exit int, args ...string) (int, string) {
	t.Helper()
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	flaky := filepath.Join(dir, "flaky")
	script := "#!/usr/bin/env bash\nprintf '%s\\n' \"$*\" >> " + calls + "\n" +
		"n=$(wc -l < " + calls + ")\n" +
		"if (( n <= " + strconv.Itoa(failures) + " )); then exit " + strconv.Itoa(exit) + "; fi\n"
	writeExecutable(t, flaky, script)
	cmd := exec.Command("bash", append([]string{"retry.sh", flaky}, args...)...)
	cmd.Env = append(os.Environ(), "RETRY_ATTEMPTS="+strconv.Itoa(attempts), "RETRY_SLEEP=0")
	err := cmd.Run()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	return code, string(log)
}

func TestRetry_runsTheCommandAgainUntilItSucceeds(t *testing.T) {
	t.Parallel()
	code, log := runRetry(t, 5, 2, 1, "pull", "an image")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if want := "pull an image\npull an image\npull an image\n"; log != want {
		t.Fatalf("calls = %q, want %q", log, want)
	}
}

func TestRetry_givesUpAfterTheLastAttemptWithTheCommandsExitCode(t *testing.T) {
	t.Parallel()
	code, log := runRetry(t, 3, 99, 7)
	if code != 7 {
		t.Fatalf("exit = %d, want 7", code)
	}
	if got := strings.Count(log, "\n"); got != 3 {
		t.Fatalf("ran %d times, want 3", got)
	}
}
