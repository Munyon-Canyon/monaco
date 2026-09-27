package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func monacoctl(t *testing.T, dir, stdin string, args ...string) (int, string, string) {
	t.Helper()
	cmd := testkit.MainCommand(t, os.Environ(), args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatalf("monacoctl %q: %v", args, err)
	}
	return cmd.ProcessState.ExitCode(), stdout.String(), stderr.String()
}

func backendRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestMain_lintCommentsDefaultsToTheWorkingTree(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte("package p\n\n// hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := monacoctl(t, dir, "", "lint", "comments")
	if code != 1 || stdout != "bad.go:3: comment not allowed\n" || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestMain_docsFlowsRendersTheRepoTSVFromTheBackendDir(t *testing.T) {
	t.Parallel()
	var want bytes.Buffer
	if code := docsFlows(os.DirFS(filepath.Join(backendRoot(t), "../..")), &want, io.Discard); code != 0 {
		t.Fatalf("docsFlows over the repo = %d", code)
	}
	code, stdout, stderr := monacoctl(t, backendRoot(t), "", "docs", "flows")
	if code != 0 || stdout != want.String() || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q, want 0 and %q", code, stdout, stderr, want.String())
	}
}

func TestMain_flowsCheckReadsTestResultsFromStdinOrFrom(t *testing.T) {
	t.Parallel()
	results := pass("TestFlow999999_NoSuchFlow")
	from := filepath.Join(t.TempDir(), "go-test.json")
	if err := os.WriteFile(from, []byte(results), 0o600); err != nil {
		t.Fatal(err)
	}
	want := "test TestFlow999999_NoSuchFlow matches no flow outcome"
	for name, run := range map[string]func() (int, string, string){
		"stdin": func() (int, string, string) { return monacoctl(t, backendRoot(t), results, "flows", "check") },
		"from": func() (int, string, string) {
			return monacoctl(t, backendRoot(t), "", "flows", "check", "--from", from)
		},
	} {
		if code, stdout, stderr := run(); code != 1 || stdout != "" || !strings.Contains(stderr, want) {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q, want 1 and %q", name, code, stdout, stderr, want)
		}
	}
}

func TestRun_unknownOrMissingCommandPrintsUsageAndExits2(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"unknown", []string{"bogus"}, "monacoctl: unknown command \"bogus\"\nusage: monacoctl <command> [args]\n  bench\n  bus\n  coverage\n  dev\n  docs\n  flows\n  gen\n  lint\n  migrate\n  mutation\n  test-report\n"},
		{"missing", nil, "usage: monacoctl <command> [args]\n  bench\n  bus\n  coverage\n  dev\n  docs\n  flows\n  gen\n  lint\n  migrate\n  mutation\n  test-report\n"},
		{"lint without subcommand", []string{"lint"}, "usage: monacoctl <command> [args]\n  comments\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if code := run(commands(), tools(nil), nil, tc.args, &stdout, &stderr); code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
			if stderr.String() != tc.want {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tc.want)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

func TestRun_dispatchesToRegisteredCommandAndListsItInUsage(t *testing.T) {
	t.Parallel()
	var got []string
	cmds := map[string]command{"echo": func(cfg config.Config, args []string, stdout, _ io.Writer) int {
		got = append([]string{string(cfg.Env)}, args...)
		_, _ = io.WriteString(stdout, strings.Join(args, " "))
		return 0
	}}

	var stdout, stderr bytes.Buffer
	if code := run(cmds, nil, validEnviron(), []string{"echo", "a", "b"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.Join(got, ",") != "test,a,b" || stdout.String() != "a b" || stderr.Len() != 0 {
		t.Fatalf("args=%v stdout=%q stderr=%q", got, stdout.String(), stderr.String())
	}

	stderr.Reset()
	run(cmds, nil, nil, nil, &stdout, &stderr)
	if want := "usage: monacoctl <command> [args]\n  echo\n"; stderr.String() != want {
		t.Fatalf("usage = %q, want %q", stderr.String(), want)
	}
}

func validEnviron() []string {
	return []string{"MONACO_ENV=test", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=nats://localhost:4222"}
}

func TestRun_commandFailsBeforeRunningWhenConfigIsInvalid(t *testing.T) {
	t.Parallel()
	ran := false
	cmds := map[string]command{"echo": func(config.Config, []string, io.Writer, io.Writer) int {
		ran = true
		return 0
	}}
	var stdout, stderr bytes.Buffer
	code := run(cmds, nil, []string{"MONACO_ENV=test", "MONACO_FOO=1"}, []string{"echo"}, &stdout, &stderr)
	want := "monacoctl: config.Load: invalid_input: missing DATABASE_URL, NATS_URL; unknown MONACO_FOO\n"
	if code != 1 || ran || stderr.String() != want || stdout.Len() != 0 {
		t.Fatalf("code=%d ran=%v stderr=%q stdout=%q", code, ran, stderr.String(), stdout.String())
	}
}

func TestRun_toolRunsWithoutLoadingConfig(t *testing.T) {
	t.Parallel()
	tls := map[string]tool{"echo": func(args []string, stdout, _ io.Writer) int {
		_, _ = io.WriteString(stdout, strings.Join(args, " "))
		return 3
	}}
	var stdout, stderr bytes.Buffer
	code := run(nil, tls, []string{"MONACO_FOO=1"}, []string{"echo", "a"}, &stdout, &stderr)
	if code != 3 || stdout.String() != "a" || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q, want 3, \"a\", no stderr", code, stdout.String(), stderr.String())
	}
}
