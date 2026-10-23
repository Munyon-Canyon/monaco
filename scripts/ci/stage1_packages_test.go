package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const packagesGoStub = `#!/bin/sh
echo "$@" >>"$STUB_LOG"
case "$*" in
*--excluded*) printf '%s' "$STUB_EXCLUDED" ;;
*) printf '%s' "$STUB_AFFECTED" ;;
esac
`

func runStage1Packages(t *testing.T, env ...string) (stdout, calls string, err error) {
	t.Helper()
	tree := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tree, "apps", "backend"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tree, "scripts", "ci"), 0o750); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("stage1-packages.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(tree, "scripts", "ci", "stage1-packages.sh")
	writeExecutable(t, script, string(src))
	stub := t.TempDir()
	writeExecutable(t, filepath.Join(stub, "go"), packagesGoStub)
	log := filepath.Join(t.TempDir(), "log")
	cmd := exec.Command(script)
	cmd.Env = append(os.Environ(), "PATH="+stub+string(os.PathListSeparator)+os.Getenv("PATH"), "STUB_LOG="+log)
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.Output()
	logged, _ := os.ReadFile(log)
	return string(out), string(logged), err
}

func TestStage1Packages_printsTheAffectedAndTheExcludedPackagesOnceEach(t *testing.T) {
	t.Parallel()
	out, calls, err := runStage1Packages(t,
		"BASE=abc123",
		"STUB_AFFECTED=./internal/b\n./internal/a\n",
		"STUB_EXCLUDED=./cmd/monacoctl\n./internal/a\n",
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := "./cmd/monacoctl\n./internal/a\n./internal/b\n"; out != want {
		t.Fatalf("packages = %q, want %q", out, want)
	}
	for _, want := range []string{"run ./cmd/monacoctl ci affected --base abc123\n", "run ./cmd/monacoctl ci affected --base abc123 --excluded\n"} {
		if !strings.Contains(calls, want) {
			t.Fatalf("go calls = %q, want %q", calls, want)
		}
	}
}

func TestStage1Packages_fullPrintsNothingAndRunsNoTool(t *testing.T) {
	t.Parallel()
	out, calls, err := runStage1Packages(t, "STAGE1_FULL=true", "STUB_AFFECTED=./internal/a\n")
	if err != nil {
		t.Fatal(err)
	}
	if out != "" || calls != "" {
		t.Fatalf("full printed %q after go calls %q, want neither", out, calls)
	}
}

func TestStage1Packages_aDiffThatReachesNoPackagePrintsNothing(t *testing.T) {
	t.Parallel()
	out, _, err := runStage1Packages(t, "BASE=abc123")
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Fatalf("packages = %q, want none", out)
	}
}

func TestStage1Packages_withoutABaseItFails(t *testing.T) {
	t.Parallel()
	if _, _, err := runStage1Packages(t); err == nil {
		t.Fatal("stage1-packages.sh without BASE exited 0, want a failure")
	}
}
