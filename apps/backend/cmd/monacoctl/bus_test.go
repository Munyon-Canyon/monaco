package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestBusApply_createsBothStreamsThenReportsNoChanges(t *testing.T) {
	t.Parallel()
	url := testkit.StandaloneNATS(t)
	environ := []string{"MONACO_ENV=test", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=" + url}
	want := []string{
		"EVENTS: created\nDEADLETTER: created\n",
		"EVENTS: no changes\nDEADLETTER: no changes\n",
	}
	for i, w := range want {
		var stdout, stderr bytes.Buffer
		if code := run(nil, tools(environ), environ, []string{"bus", "apply"}, &stdout, &stderr); code != 0 {
			t.Fatalf("run %d: exit %d, stderr %q", i+1, code, stderr.String())
		}
		if stdout.String() != w || stderr.Len() != 0 {
			t.Fatalf("run %d: stdout %q stderr %q, want stdout %q", i+1, stdout.String(), stderr.String(), w)
		}
	}
}

func TestBusApply_failsWhenNATSIsUnreachableOrArgsAreExtra(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		url  string
		args []string
		code int
		want string
	}{
		{"unreachable", "nats://127.0.0.1:1", []string{"bus", "apply"}, 1, "monacoctl: bus.Connect: upstream_unavailable"},
		{"extra args", testkit.NATSURL(), []string{"bus", "apply", "now"}, 2, "usage: monacoctl bus apply\n"},
		{"no subcommand", testkit.NATSURL(), []string{"bus"}, 2, "usage: monacoctl <command> [args]\n  apply\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			environ := []string{"MONACO_ENV=test", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=" + tc.url}
			var stdout, stderr bytes.Buffer
			if code := run(nil, tools(environ), environ, tc.args, &stdout, &stderr); code != tc.code {
				t.Fatalf("exit %d, want %d; stderr %q", code, tc.code, stderr.String())
			}
			if !bytes.HasPrefix(stderr.Bytes(), []byte(tc.want)) || stdout.Len() != 0 {
				t.Fatalf("stdout %q stderr %q, want stderr starting %q", stdout.String(), stderr.String(), tc.want)
			}
		})
	}
}

func TestBusApply_failsWhenAForeignStreamHoldsTheSubjects(t *testing.T) {
	t.Parallel()
	url := testkit.StandaloneNATSWithStream(t, "FOREIGN", "events.>")
	environ := []string{"MONACO_ENV=test", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=" + url}
	var stdout, stderr bytes.Buffer
	code := run(nil, tools(environ), environ, []string{"bus", "apply"}, &stdout, &stderr)
	if code != 1 || !strings.HasPrefix(stderr.String(), "monacoctl: bus.Apply: upstream_unavailable") {
		t.Fatalf("exit %d, stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
}
