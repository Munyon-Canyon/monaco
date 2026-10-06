package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const mobileCoreFakeSwift = `#!/bin/sh
bin="$PWD/.build/debug"
if [ "$2" = --show-codecov-path ]; then echo "$bin/codecov/MonacoCore.json"; exit 0; fi
mkdir -p "$bin/codecov" "$bin/Dir.xctest/Contents/MacOS"
: > "$bin/Dir.xctest/Contents/MacOS/Dir"
: > "$bin/File.xctest"
printf "Test Case '-[A.B testOne]' started.\n"
printf "Test Case '-[A.B testOne]' pa"
printf "warning: '--build-system native' has been deprecated\n" >&2
printf "ssed (0.010 seconds).\n"
printf "Test Case 'A.testTwo' started at 2026-10-02 12:00:00.000\n"
printf "Test Case 'A.testTwo' passed (0.020 seconds)\n"
printf '\342\234\224 Test "three" passed after 0.030 seconds.\n'
`

const mobileCoreFakeLLVMCov = `#!/bin/sh
[ "$1" = llvm-cov ] && shift
echo "$@" > "$STUB_ARGS"
echo "TOTAL 1 2 3 4 5 6 7 8 90.50%"
`

type mobileCoreRun struct {
	dir, args string
	out       string
	err       error
}

func runMobileCoreTest(t *testing.T, floor string) mobileCoreRun {
	t.Helper()
	return runMobileCoreTestWith(t, floor)
}

func runMobileCoreTestWith(t *testing.T, floor string, args ...string) mobileCoreRun {
	t.Helper()
	root := repoRoot(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	copyFile(t, filepath.Join(root, "scripts/mobile-core-test.sh"), filepath.Join(dir, "scripts/mobile-core-test.sh"))
	writeRatchetFile(t, filepath.Join(dir, "packages/mobile-core/coverage-floor.txt"), floor)
	bin := t.TempDir()
	writeRatchetFile(t, filepath.Join(bin, "swift"), mobileCoreFakeSwift)
	writeRatchetFile(t, filepath.Join(bin, "xcrun"), mobileCoreFakeLLVMCov)
	writeRatchetFile(t, filepath.Join(bin, "llvm-cov"), mobileCoreFakeLLVMCov)
	argsFile := filepath.Join(t.TempDir(), "args")

	cmd := exec.Command("bash", append([]string{"scripts/mobile-core-test.sh"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin", "STUB_ARGS="+argsFile)
	out, err := cmd.CombinedOutput()
	return mobileCoreRun{dir: dir, args: argsFile, out: string(out), err: err}
}

func (r mobileCoreRun) floor(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(r.dir, "packages/mobile-core/coverage-floor.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func hostPlatform() (self, other string) {
	if runtime.GOOS == "darwin" {
		return "darwin", "linux"
	}
	return "linux", "darwin"
}

func TestMobileCoreTest_coverageWellAboveTheFloorPassesWithoutEditingTheFloor(t *testing.T) {
	self, other := hostPlatform()
	floor := self + " 89.00\n" + other + " 70.00\n"
	run := runMobileCoreTest(t, floor)
	if run.err != nil {
		t.Fatalf("a coverage rise failed the run: %v\n%s", run.err, run.out)
	}
	if want := "coverage rose: " + self + " 89.00 -> 90.50"; !strings.Contains(run.out, want) {
		t.Fatalf("missing %q in:\n%s", want, run.out)
	}
	if got := run.floor(t); got != floor {
		t.Fatalf("a plain run edited coverage-floor.txt to %q", got)
	}
}

func TestMobileCoreTest_updateFloorRaisesOnlyThisPlatformsRow(t *testing.T) {
	self, other := hostPlatform()
	run := runMobileCoreTestWith(t, self+" 89.00\n"+other+" 70.00\n", "--update-floor")
	if run.err != nil {
		t.Fatalf("--update-floor: %v\n%s", run.err, run.out)
	}
	rows := map[string]string{self: "90.50", other: "70.00"}
	if got, want := run.floor(t), "darwin "+rows["darwin"]+"\nlinux "+rows["linux"]+"\n"; got != want {
		t.Fatalf("coverage-floor.txt\n got %q\nwant %q", got, want)
	}
}

func TestMobileCoreTest_updateFloorRefusesToLowerIt(t *testing.T) {
	self, other := hostPlatform()
	floor := self + " 90.60\n" + other + " 70.00\n"
	run := runMobileCoreTestWith(t, floor, "--update-floor")
	if run.err == nil || !strings.Contains(run.out, "refusing to lower the floor") {
		t.Fatalf("--update-floor lowered the floor: %v\n%s", run.err, run.out)
	}
	if got := run.floor(t); got != floor {
		t.Fatalf("coverage-floor.txt changed to %q", got)
	}
}

func TestMobileCoreTest_coverageJustAboveTheFloorLeavesTheFloorAlone(t *testing.T) {
	self, other := hostPlatform()
	floor := self + " 90.00\n" + other + " 70.00\n"
	run := runMobileCoreTest(t, floor)
	if run.err != nil {
		t.Fatalf("mobile-core-test.sh: %v\n%s", run.err, run.out)
	}
	if strings.Contains(run.out, "coverage rose") {
		t.Fatalf("a rise inside the 0.5 margin raised the floor:\n%s", run.out)
	}
	if got := run.floor(t); got != floor {
		t.Fatalf("coverage-floor.txt changed to %q", got)
	}
}

func TestMobileCoreTest_coverageBelowTheFloorFailsAndKeepsTheFloor(t *testing.T) {
	self, other := hostPlatform()
	floor := self + " 90.60\n" + other + " 70.00\n"
	run := runMobileCoreTest(t, floor)
	if run.err == nil {
		t.Fatalf("coverage under the floor passed:\n%s", run.out)
	}
	if want := "coverage fell: " + self + " 90.60 -> 90.50"; !strings.Contains(run.out, want) {
		t.Fatalf("missing %q in:\n%s", want, run.out)
	}
	if got := run.floor(t); got != floor {
		t.Fatalf("coverage-floor.txt changed to %q", got)
	}
}

func TestMobileCoreTest_stderrWrittenMidLineLeavesTheTimingsParseable(t *testing.T) {
	run := runMobileCoreTest(t, "darwin 90.50\nlinux 90.50\n")
	if run.err != nil {
		t.Fatalf("mobile-core-test.sh: %v\n%s", run.err, run.out)
	}
	dir, args := run.dir, run.args
	for _, want := range []string{`slowest test: "three" 0.03s (budget 2 s)`, "coverage: "} {
		if !strings.Contains(run.out, want) {
			t.Fatalf("missing %q in:\n%s", want, run.out)
		}
	}
	parsed, err := os.ReadFile(filepath.Join(dir, "packages/mobile-core/.build/test-output.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(parsed), "warning:") {
		t.Fatalf("stderr reached the parsed test output:\n%s", parsed)
	}
	got, err := os.ReadFile(args)
	if err != nil {
		t.Fatal(err)
	}
	debug := filepath.Join(dir, "packages/mobile-core/.build/debug")
	want := "report " + debug + "/Dir.xctest/Contents/MacOS/Dir -object " + debug + "/File.xctest -instr-profile " +
		debug + "/codecov/default.profdata -ignore-filename-regex=(\\.build|Tests)/"
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("llvm-cov args\n got %q\nwant %q", strings.TrimSpace(string(got)), want)
	}
}
