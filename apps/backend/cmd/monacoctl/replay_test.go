package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	seededPing  = "01890a5d-ac96-774b-bcce-b302099a8059"
	unreachable = "postgres://monaco@localhost:1/monaco?sslmode=disable&connect_timeout=1"
)

func opsEnv(url string) []string {
	return []string{"MONACO_ENV=test", "DATABASE_URL=" + url, "NATS_URL=nats://127.0.0.1:1"}
}

func runOps(environ []string, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := run(nil, tools(environ), environ, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func seededFlow00(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testkit.DB(t)
	testkit.Seed(t, pool, "one-user-with-ping")
	return pool
}

func TestBackfillThenReplay_verifyReportsZeroDiffsOnFlow00State(t *testing.T) {
	t.Parallel()
	source := seededFlow00(t)
	env := opsEnv(source.Config().ConnString())
	for _, want := range []string{
		"backfill system.echo: 1 events, 1 applied, 0 duplicate\n",
		"backfill system.echo: 1 events, 0 applied, 1 duplicate\n",
	} {
		code, stdout, stderr := runOps(env, "backfill", "--consumer", "system.echo", "--types", "system.pinged")
		if code != 0 || stdout != want || stderr != "" {
			t.Fatalf("backfill: code=%d stdout=%q stderr=%q, want %q", code, stdout, stderr, want)
		}
	}
	t.Run("target", func(t *testing.T) {
		t.Parallel()
		into := testkit.DB(t).Config().ConnString()
		code, stdout, stderr := runOps(env, "replay", "--into", into, "--to", seededPing, "--verify")
		if code != 0 || stdout != "replayed 1 events: 1 applied, 0 duplicate\nverify: 0 diffs\n" || stderr != "" {
			t.Fatalf("replay --verify: code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		t.Logf("monacoctl replay --to %s --verify:\n%s", seededPing, stdout)
		code, stdout, stderr = runOps(env, "replay", "--into", into)
		if code != 1 || stdout != "replayed 0 events: 0 applied, 0 duplicate\n" ||
			!strings.HasSuffix(stderr, "target holds 1 events; replay writes only into a fresh database\n") {
			t.Fatalf("second replay: code=%d stdout=%q stderr=%q, want a refusal", code, stdout, stderr)
		}
	})
}

func TestReplay_verifyExitsOneAndPrintsEachDiffWhenTheSourceWasNeverEchoed(t *testing.T) {
	t.Parallel()
	source := seededFlow00(t)
	env := opsEnv(source.Config().ConnString())
	t.Run("target", func(t *testing.T) {
		t.Parallel()
		code, stdout, stderr := runOps(env, "replay", "--into", testkit.DB(t).Config().ConnString(), "--verify")
		lines := strings.Split(stdout, "\n")
		if code != 1 || len(lines) != 4 || !strings.HasPrefix(lines[1], "system_pings: only in replay: ") ||
			lines[2] != "verify: 1 diffs" || stderr != "" {
			t.Fatalf("code=%d stdout=%q stderr=%q, want the unechoed source's diff and exit 1", code, stdout, stderr)
		}
	})
}

func TestReplayAndBackfill_refuseBadArgumentsAndUnusableDatabases(t *testing.T) {
	t.Parallel()
	source := testkit.DB(t).Config().ConnString()
	env := opsEnv(source)
	for _, tc := range []struct {
		name    string
		environ []string
		args    []string
		code    int
		stderr  string
	}{
		{"replay needs --into", env, []string{"replay"}, 2, replayUsage + "\n"},
		{"replay --to is an id", env, []string{"replay", "--into", source, "--to", "nope"}, 2, replayUsage + "\n"},
		{"replay takes no args", env, []string{"replay", "--into", source, "now"}, 2, replayUsage + "\n"},
		{"replay needs config", nil, []string{"replay", "--into", source}, 1, "monacoctl: "},
		{
			"replay refuses the source", env,
			[]string{"replay", "--into", source},
			1,
			"monacoctl: replay.CheckTarget: invalid_input: target is the source database\n",
		},
		{
			"replay refuses dev", env,
			[]string{"replay", "--into", "postgres://monaco@localhost:54322/monaco"},
			1,
			"monacoctl: replay.CheckTarget: invalid_input: target is on the dev database container",
		},
		{"replay target unreachable", env, []string{"replay", "--into", unreachable}, 1, "monacoctl: db.Open: "},
		{"backfill needs flags", env, []string{"backfill", "--consumer", "system.echo"}, 2, backfillUsage + "\n"},
		{"backfill --since is an id", env, []string{
			"backfill", "--consumer", "system.echo", "--types", "system.pinged", "--since", "nope",
		}, 2, backfillUsage + "\n"},
		{"backfill needs config", nil, []string{"backfill", "--consumer", "x", "--types", "y"}, 1, "monacoctl: "},
		{
			"backfill db unreachable", opsEnv(unreachable),
			[]string{"backfill", "--consumer", "x", "--types", "y"},
			1,
			"monacoctl: db.Open: ",
		},
		{
			"backfill unknown handler", env,
			[]string{"backfill", "--consumer", "nope", "--types", "system.pinged"},
			1,
			"monacoctl: no registered handler \"nope\"\n",
		},
		{
			"backfill wrong type", env,
			[]string{"backfill", "--consumer", "system.echo", "--types", "cabal.created"},
			1,
			"monacoctl: replay.Backfill: invalid_input: handler system.echo handles system.pinged, not cabal.created\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, _, stderr := runOps(tc.environ, tc.args...)
			if code != tc.code || !strings.HasPrefix(stderr, tc.stderr) {
				t.Fatalf("code=%d stderr=%q, want %d and a stderr starting %q", code, stderr, tc.code, tc.stderr)
			}
		})
	}
}
