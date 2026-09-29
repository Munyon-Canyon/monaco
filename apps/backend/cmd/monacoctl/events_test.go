package main

import (
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestEventsExport_writesTheLogAsSeedLines(t *testing.T) {
	t.Parallel()
	env := opsEnv(seededFlow00(t).Config().ConnString())
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"events", "export"}, `"actor":"user:01890a5d-ac96-774b-bcce-b302099a8058"`},
		{[]string{"events", "export", "--aggregate", "system:01890a5d-ac96-774b-bcce-b302099a8057"}, `"note":"seeded"`},
		{[]string{"events", "export", "--anonymize"}, `"ping_id":"01890a5d-ac96-774b-bcce-b302099a8057"`},
	} {
		code, stdout, stderr := runOps(env, tc.args...)
		if code != 0 || strings.Count(stdout, "\n") != 1 || !strings.Contains(stdout, tc.want) || stderr != "" {
			t.Fatalf("%q: code=%d stdout=%q stderr=%q, want one line with %s", tc.args, code, stdout, stderr, tc.want)
		}
	}
}

func TestEventsExport_refusesBadArgumentsAndAnUnreachableDatabase(t *testing.T) {
	t.Parallel()
	env := opsEnv(testkit.DB(t).Config().ConnString())
	for _, tc := range []struct {
		environ []string
		args    []string
		code    int
		stderr  string
	}{
		{env, []string{"events", "export", "now"}, 2, eventsUsage + "\n"},
		{env, []string{"events", "export", "--aggregate", "system"}, 2, eventsUsage + "\n"},
		{env, []string{"events", "export", "--aggregate", ":01890a5d-ac96-774b-bcce-b302099a8057"}, 2, eventsUsage + "\n"},
		{opsEnv(unreachable), []string{"events", "export"}, 1, "monacoctl: db.Open: "},
	} {
		code, _, stderr := runOps(tc.environ, tc.args...)
		if code != tc.code || !strings.HasPrefix(stderr, tc.stderr) {
			t.Errorf("%q: code=%d stderr=%q, want %d and %q", tc.args, code, stderr, tc.code, tc.stderr)
		}
	}
}
