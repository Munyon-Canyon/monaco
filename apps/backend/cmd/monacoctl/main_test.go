package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestRun_unknownOrMissingCommandPrintsUsageAndExits2(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"unknown", []string{"bogus"}, "monacoctl: unknown command \"bogus\"\nusage: monacoctl <command> [args]\n"},
		{"missing", nil, "usage: monacoctl <command> [args]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if code := run(commands(), tc.args, &stdout, &stderr); code != 2 {
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
	cmds := map[string]command{"echo": func(args []string, stdout, _ io.Writer) int {
		got = args
		_, _ = io.WriteString(stdout, strings.Join(args, " "))
		return 0
	}}

	var stdout, stderr bytes.Buffer
	if code := run(cmds, []string{"echo", "a", "b"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.Join(got, ",") != "a,b" || stdout.String() != "a b" || stderr.Len() != 0 {
		t.Fatalf("args=%v stdout=%q stderr=%q", got, stdout.String(), stderr.String())
	}

	stderr.Reset()
	run(cmds, nil, &stdout, &stderr)
	if want := "usage: monacoctl <command> [args]\n  echo\n"; stderr.String() != want {
		t.Fatalf("usage = %q, want %q", stderr.String(), want)
	}
}
