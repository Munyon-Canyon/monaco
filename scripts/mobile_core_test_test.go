package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
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

func TestMobileCoreTest_stderrWrittenMidLineLeavesTheTimingsParseable(t *testing.T) {
	root := repoRoot(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	copyFile(t, filepath.Join(root, "scripts/mobile-core-test.sh"), filepath.Join(dir, "scripts/mobile-core-test.sh"))
	writeRatchetFile(t, filepath.Join(dir, "packages/mobile-core/coverage-floor.txt"), "darwin 90.50\nlinux 90.50\n")
	bin := t.TempDir()
	writeRatchetFile(t, filepath.Join(bin, "swift"), mobileCoreFakeSwift)
	writeRatchetFile(t, filepath.Join(bin, "xcrun"), mobileCoreFakeLLVMCov)
	writeRatchetFile(t, filepath.Join(bin, "llvm-cov"), mobileCoreFakeLLVMCov)
	args := filepath.Join(t.TempDir(), "args")

	cmd := exec.Command("bash", "scripts/mobile-core-test.sh")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin", "STUB_ARGS="+args)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mobile-core-test.sh: %v\n%s", err, out)
	}
	for _, want := range []string{`slowest test: "three" 0.03s (budget 2 s)`, "coverage: "} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("missing %q in:\n%s", want, out)
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
