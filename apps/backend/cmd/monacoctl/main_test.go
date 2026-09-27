package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestRun_unknownOrMissingCommandPrintsUsageAndExits2(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"unknown", []string{"bogus"}, "monacoctl: unknown command \"bogus\"\nusage: monacoctl <command> [args]\n"},
		{"missing", nil, "usage: monacoctl <command> [args]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tc.args, &stdout, &stderr); code != 2 {
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

func TestRun_dispatchesToRegisteredCommand(t *testing.T) {
	var got []string
	commands["echo"] = func(args []string, stdout, _ io.Writer) int {
		got = args
		_, _ = io.WriteString(stdout, strings.Join(args, " "))
		return 0
	}
	t.Cleanup(func() { delete(commands, "echo") })

	var stdout, stderr bytes.Buffer
	if code := run([]string{"echo", "a", "b"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.Join(got, ",") != "a,b" || stdout.String() != "a b" || stderr.Len() != 0 {
		t.Fatalf("args=%v stdout=%q stderr=%q", got, stdout.String(), stderr.String())
	}
}
