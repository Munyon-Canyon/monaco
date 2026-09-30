package ci_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestJobSummary_timingTestsAndFailureTail(t *testing.T) {
	root := filepath.Join("..", "..")
	script := filepath.Join(root, "scripts", "ci", "job-summary.sh")
	dir := t.TempDir()

	build := filepath.Join(dir, "build.log")
	if err := os.WriteFile(build, []byte(strings.Join([]string{
		"noise before",
		"Build Timing Summary",
		"SwiftCompile (48 tasks) | 10.000 seconds",
		"Ld (3 tasks) | 30.500 seconds",
		"SwiftCompile normal arm64 Compiling Foo.swift (in target 'Monaco' from project 'Monaco')",
		"    98.250 seconds",
		"Copy (1 task) | 0.050 seconds",
		"** BUILD SUCCEEDED **",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := filepath.Join(dir, "tests.log")
	if err := os.WriteFile(tests, []byte(strings.Join([]string{
		"✔ Test example() passed after 0.001 seconds.",
		"✔ Suite Big passed after 9.000 seconds.",
		"✔ Test slowOne() passed after 3.500 seconds.",
		"Test Case '-[MonacoTests.Foo testBar]' passed (1.250 seconds).",
		"✔ Test medium() passed after 2.000 seconds.",
		"✔ exampleCLI() (4.000 seconds)",
		"✔ Test slowOne() passed after 0.010 seconds.",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	times := filepath.Join(dir, "times.tsv")
	if err := os.WriteFile(times, []byte("Resolve Swift packages\t12\nBuild app and tests\t500\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var raw strings.Builder
	for i := 1; i <= 70; i++ {
		fmt.Fprintf(&raw, "line %d\n", i)
	}
	failLog := filepath.Join(dir, "fail.log")
	if err := os.WriteFile(failLog, []byte(raw.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	failList := filepath.Join(dir, "fail.tsv")
	if err := os.WriteFile(failList, []byte("Build app and tests\t"+failLog+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"STEP_TIMES="+times,
		"BUILD_LOG="+build,
		"TEST_LOG="+tests,
		"FAIL_LOGS="+failList,
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
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
		"Last 60 lines of Build app and tests",
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
	slowIdx := strings.Index(text, "98.250 seconds")
	ldIdx := strings.Index(text, "30.500 seconds")
	if slowIdx < 0 || ldIdx < slowIdx {
		t.Fatalf("timing lines are not ordered by time\n%s", text)
	}
	firstSlow := strings.Index(text, "4.000\texampleCLI()")
	secondSlow := strings.Index(text, "3.500\tslowOne()")
	if firstSlow < 0 || secondSlow < firstSlow {
		t.Fatalf("tests are not ordered by time\n%s", text)
	}
}
