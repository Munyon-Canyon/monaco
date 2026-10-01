package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestJobSummary_timingTestsAndFailureTail(t *testing.T) {
	root := filepath.Join("..", "..")
	script := filepath.Join(root, "scripts", "ci", "job-summary.sh")
	data := filepath.Join(root, "scripts", "ci", "testdata")

	text := runSummary(t, script, []string{
		"STEP_TIMES=" + filepath.Join(data, "job-summary-times.tsv"),
		"BUILD_LOG=" + filepath.Join(data, "job-summary-build.log"),
		"TEST_LOG=" + filepath.Join(data, "job-summary-tests.log"),
		"FAIL_LOGS=" + failList(t, filepath.Join(data, "job-summary-fail.log")),
	})
	for _, want := range []string{
		"Resolve Swift packages\t12",
		"Build app and tests\t500",
		"98.250 seconds",
		"30.500 seconds",
		"10.000 seconds",
		"4.000\texampleCLI()",
		"3.500\tslowOne()",
		"2.000\tmedium()",
		"1.250\t-[MonacoTests.Foo testBar]",
		"<details>",
		"<summary>Last 60 lines of Build app and tests</summary>",
		"line 11",
		"line 70",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("summary missing %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "Suite Big") {
		t.Fatalf("suite duration leaked into slowest tests\n%s", text)
	}
	if strings.Contains(text, "line 10\n") {
		t.Fatalf("failure tail kept more than 60 lines\n%s", text)
	}
	if got := failureTailLines(t, text); got != 60 {
		t.Fatalf("failure tail has %d lines, want 60\n%s", got, text)
	}
	if strings.Index(text, "98.250 seconds") > strings.Index(text, "30.500 seconds") {
		t.Fatalf("timing lines are not ordered by time\n%s", text)
	}
	if strings.Index(text, "4.000\texampleCLI()") > strings.Index(text, "3.500\tslowOne()") {
		t.Fatalf("tests are not ordered by time\n%s", text)
	}

	passing := runSummary(t, script, []string{
		"STEP_TIMES=" + filepath.Join(data, "job-summary-times.tsv"),
		"BUILD_LOG=" + filepath.Join(data, "job-summary-build.log"),
		"TEST_LOG=" + filepath.Join(data, "job-summary-tests.log"),
	})
	if strings.Contains(passing, "<details>") {
		t.Fatalf("passing run included a failure tail\n%s", passing)
	}
}

func TestJobSummary_stripsLogTimestamps(t *testing.T) {
	root := filepath.Join("..", "..")
	script := filepath.Join(root, "scripts", "ci", "job-summary.sh")
	data := filepath.Join(root, "scripts", "ci", "testdata")
	dir := t.TempDir()
	buildLog := filepath.Join(dir, "build.log")
	testLog := filepath.Join(dir, "tests.log")
	prefixFile(t, filepath.Join(data, "job-summary-build.log"), buildLog)
	prefixFile(t, filepath.Join(data, "job-summary-tests.log"), testLog)

	text := runSummary(t, script, []string{
		"STEP_TIMES=" + filepath.Join(data, "job-summary-times.tsv"),
		"BUILD_LOG=" + buildLog,
		"TEST_LOG=" + testLog,
	})
	for _, want := range []string{
		"98.250 seconds",
		"30.500 seconds",
		"4.000\texampleCLI()",
		"1.250\t-[MonacoTests.Foo testBar]",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("summary missing %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "12:00:00") {
		t.Fatalf("timestamp leaked into the summary\n%s", text)
	}
}

func prefixFile(t *testing.T, src, dst string) {
	t.Helper()
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		b.WriteString("12:00:00 ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(dst, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runSummary(t *testing.T, script string, env []string) string {
	t.Helper()
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func failList(t *testing.T, logPath string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fail.tsv")
	if err := os.WriteFile(path, []byte("Build app and tests\t"+logPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func failureTailLines(t *testing.T, text string) int {
	t.Helper()
	const open = "<summary>Last 60 lines of Build app and tests</summary>"
	i := strings.Index(text, open)
	if i < 0 {
		t.Fatal("missing failure details")
	}
	rest := text[i+len(open):]
	fence := strings.Index(rest, "```\n")
	if fence < 0 {
		t.Fatal("missing opening fence")
	}
	rest = rest[fence+4:]
	end := strings.Index(rest, "```")
	if end < 0 {
		t.Fatal("missing closing fence")
	}
	body := strings.Trim(rest[:end], "\n")
	if body == "" {
		return 0
	}
	return len(strings.Split(body, "\n"))
}
