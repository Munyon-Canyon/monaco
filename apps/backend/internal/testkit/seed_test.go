package testkit_test

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func goTest(t *testing.T, args ...string) (string, error) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs another test binary; CI runs it without -short, outside the 10 s package budget")
	}
	cmd := exec.CommandContext(t.Context(), "go", append([]string{"test", "-count=1", "-v"}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestRandSeedLogsOnlyFailuresAndReplays(t *testing.T) {
	t.Parallel()
	out, err := goTest(t, "./testdata/seed")
	if err == nil {
		t.Fatalf("fixture with a failing test passed:\n%s", out)
	}
	failed, passed, _ := strings.Cut(out, "=== RUN   TestPasses")
	m := regexp.MustCompile(`seed=(\d+)`).FindStringSubmatch(failed)
	if m == nil || !strings.Contains(failed, "rerun with -testkit.seed="+m[1]) {
		t.Fatalf("failed test did not log its seed:\n%s", out)
	}
	if !strings.Contains(passed, "seed=") || strings.Contains(passed, "rerun with") {
		t.Fatalf("passing test logged a rerun line:\n%s", out)
	}
	replay, err := goTest(t, "./testdata/seed", "-run", "^TestFails$", "-args", "-testkit.seed="+m[1])
	if err == nil || !strings.Contains(replay, "seed="+m[1]+"\n") {
		t.Fatalf("-testkit.seed=%s did not replay that seed:\n%s", m[1], replay)
	}
}
