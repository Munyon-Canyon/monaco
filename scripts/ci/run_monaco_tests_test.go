package ci_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const retryMessage = "MonacoTests hung for 12 minutes, shutting the simulator down and retrying once"

// runMonacoTests runs run-monaco-tests.sh with a fake xcrun that logs its arguments and the given
// test command, killed after one second per attempt.
func runMonacoTests(t *testing.T, testCmd string) (int, string, string) {
	t.Helper()
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	xcrun := filepath.Join(dir, "xcrun")
	writeExecutable(t, xcrun, "#!/usr/bin/env bash\nprintf '%s\\n' \"$*\" >> "+calls+"\n"+
		"[[ \"$2\" == list ]] && echo '{\"devices\":{\"iOS\":[{\"udid\":\"SIM-UDID\",\"state\":\"Shutdown\"}]}}'\nexit 0\n")
	if err := os.WriteFile(filepath.Join(dir, "sim"), []byte("SIM-UDID\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "run-monaco-tests.sh")
	cmd.Env = append(os.Environ(),
		"RUNNER_TEMP="+dir,
		"MONACO_IOS_XCRUN="+xcrun,
		"MONACO_IOS_TEST_CMD="+testCmd,
		"MONACO_IOS_TEST_TIMEOUT=1",
	)
	out, err := cmd.CombinedOutput()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	log, _ := os.ReadFile(calls)
	return code, string(out), string(log)
}

func TestRunMonacoTests_retriesOnceAfterAHangAndRestartsTheSimulator(t *testing.T) {
	t.Parallel()
	marker := filepath.Join(t.TempDir(), "ran")
	code, out, calls := runMonacoTests(t, `if [ -e "`+marker+`" ]; then exit 0; fi; touch "`+marker+`"; exec sleep 2000`)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out)
	}
	if !strings.Contains(out, retryMessage) {
		t.Fatalf("output lacks the retry message:\n%s", out)
	}
	want := "simctl bootstatus SIM-UDID -b\nsimctl shutdown SIM-UDID\nsimctl boot SIM-UDID\nsimctl bootstatus SIM-UDID -b\nsimctl shutdown SIM-UDID\nsimctl list devices -j\n"
	if calls != want {
		t.Fatalf("xcrun calls = %q, want %q", calls, want)
	}
}

func TestRunMonacoTests_failsOnASecondHang(t *testing.T) {
	t.Parallel()
	code, out, _ := runMonacoTests(t, "exec sleep 2000")
	if code == 0 {
		t.Fatalf("exit = 0, want a failure\n%s", out)
	}
	if got := strings.Count(out, retryMessage); got != 1 {
		t.Fatalf("retry message printed %d times, want 1\n%s", got, out)
	}
}

func TestRunMonacoTests_doesNotRetryATestFailure(t *testing.T) {
	t.Parallel()
	code, out, calls := runMonacoTests(t, "exit 65")
	if code != 65 {
		t.Fatalf("exit = %d, want 65\n%s", code, out)
	}
	if strings.Contains(out, retryMessage) || strings.Count(calls, "boot SIM-UDID") != 0 {
		t.Fatalf("a failing test must not restart the simulator:\n%s\n%s", out, calls)
	}
	if got := strings.Count(calls, "simctl shutdown SIM-UDID"); got != 1 {
		t.Fatalf("shutdown called %d times, want 1 after the tests:\n%s", got, calls)
	}
	if !strings.Contains(out, "simulator shut down in ") {
		t.Fatalf("output lacks the shutdown message:\n%s", out)
	}
}
