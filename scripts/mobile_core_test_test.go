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
if [ -n "$STUB_TEST_SECS" ]; then
printf "Test Case 'A.testSlow' started at 2026-10-02 12:00:00.000\n"
printf "Test Case 'A.testSlow' passed (%s seconds)\n" "$STUB_TEST_SECS"
exit 0
fi
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

// mobileCoreLoad is the stubbed machine: the load1 and core count the script reads through
// sysctl (Darwin) or cat (Linux).
type mobileCoreLoad struct{ load1, cores string }

var mobileCoreIdleMachine = mobileCoreLoad{load1: "0.50", cores: "8"}

const mobileCoreFakeSysctl = `#!/bin/sh
case "$2" in
vm.loadavg) echo "{ $STUB_LOAD 1.00 1.00 }" ;;
hw.ncpu) echo "$STUB_CORES" ;;
*) exit 1 ;;
esac
`

const mobileCoreFakeCat = `#!/bin/sh
if [ "$1" = /proc/loadavg ]; then echo "$STUB_LOAD 1.00 1.00 1/100 1234"; exit 0; fi
if [ "$1" = /proc/cpuinfo ]; then i=0; while [ "$i" -lt "$STUB_CORES" ]; do echo "processor : $i"; i=$((i+1)); done; exit 0; fi
exec /bin/cat "$@"
`

func runMobileCoreTestWith(t *testing.T, floor string, args ...string) mobileCoreRun {
	t.Helper()
	return runMobileCoreTestLoaded(t, floor, mobileCoreIdleMachine, nil, args...)
}

// runMobileCoreTestLoaded drops GITHUB_ACTIONS from the inherited env; extra may set it back.
func runMobileCoreTestLoaded(t *testing.T, floor string, m mobileCoreLoad, extra []string, args ...string) mobileCoreRun {
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
	writeRatchetFile(t, filepath.Join(bin, "sysctl"), mobileCoreFakeSysctl)
	writeRatchetFile(t, filepath.Join(bin, "cat"), mobileCoreFakeCat)
	argsFile := filepath.Join(t.TempDir(), "args")

	cmd := exec.Command("bash", append([]string{"scripts/mobile-core-test.sh"}, args...)...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GITHUB_ACTIONS=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "PATH="+bin+":/usr/bin:/bin", "STUB_ARGS="+argsFile, "MONACO_SWIFTPM_LOCKED=1",
		"STUB_LOAD="+m.load1, "STUB_CORES="+m.cores)
	cmd.Env = append(cmd.Env, extra...)
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

func slowTestRun(t *testing.T, secs string, m mobileCoreLoad, gha bool) mobileCoreRun {
	t.Helper()
	extra := []string{"STUB_TEST_SECS=" + secs}
	if gha {
		extra = append(extra, "GITHUB_ACTIONS=true")
	}
	return runMobileCoreTestLoaded(t, "darwin 90.50\nlinux 90.50\n", m, extra)
}

func TestMobileCoreTest_loadScalesThePerTestBudgetUpToFourTimes(t *testing.T) {
	for _, load1 := range []string{"32.00", "80.00"} {
		m := mobileCoreLoad{load1: load1, cores: "8"}
		line := "per-test budget: 8 s (2 s x 4.0, load " + load1[:len(load1)-1] + " on 8 cores)"
		if run := slowTestRun(t, "7.000", m, false); run.err != nil || !strings.Contains(run.out, line) {
			t.Fatalf("load %s: a 7 s test under the 8 s budget: %v\n%s", load1, run.err, run.out)
		}
		run := slowTestRun(t, "9.000", m, false)
		if run.err == nil || !strings.Contains(run.out, "slow test: A.testSlow 9.000s (budget 8 s)") {
			t.Fatalf("load %s: a 9 s test passed an 8 s budget: %v\n%s", load1, run.err, run.out)
		}
	}
}

func TestMobileCoreTest_anIdleMachineKeepsTheTwoSecondBudget(t *testing.T) {
	run := slowTestRun(t, "2.500", mobileCoreIdleMachine, false)
	if run.err == nil || !strings.Contains(run.out, "slow test: A.testSlow 2.500s (budget 2 s)") {
		t.Fatalf("a 2.5 s test passed an idle machine: %v\n%s", run.err, run.out)
	}
	if want := "per-test budget: 2 s (2 s x 1.0, load 0.5 on 8 cores)"; !strings.Contains(run.out, want) {
		t.Fatalf("missing %q in:\n%s", want, run.out)
	}
}

func TestMobileCoreTest_githubActionsNeverScalesTheBudget(t *testing.T) {
	run := slowTestRun(t, "2.500", mobileCoreLoad{load1: "80.00", cores: "8"}, true)
	if run.err == nil || !strings.Contains(run.out, "slow test: A.testSlow 2.500s (budget 2 s)") {
		t.Fatalf("a 2.5 s test passed under GITHUB_ACTIONS at high load: %v\n%s", run.err, run.out)
	}
	if want := "per-test budget: 2 s (2 s x 1.0, no load applied)"; !strings.Contains(run.out, want) {
		t.Fatalf("missing %q in:\n%s", want, run.out)
	}
}

func TestMobileCoreTest_unreadableLoadFallsBackToFactorOne(t *testing.T) {
	run := slowTestRun(t, "2.500", mobileCoreLoad{load1: "garbage", cores: "8"}, false)
	if run.err == nil || !strings.Contains(run.out, "per-test budget: 2 s (2 s x 1.0, no load applied)") {
		t.Fatalf("an unreadable load did not fall back to factor 1: %v\n%s", run.err, run.out)
	}
}
