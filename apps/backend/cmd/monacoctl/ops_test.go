package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seededOps(t *testing.T) (tool, ids.CabalID, *pgxpool.Pool) {
	t.Helper()
	pool := testkit.DB(t)
	cabal := testkit.NewCabal(t, pool)
	environ := []string{"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://unused"}
	return opsTool(environ, testkit.NewClock(clock.Real{}.Now().UTC())), cabal.ID, pool
}

func runOpsTool(t *testing.T, ops tool, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := ops(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestOps_rejectsMalformedArgumentsWithTheUsage(t *testing.T) {
	t.Parallel()
	ops, cabal, _ := seededOps(t)
	for _, args := range [][]string{
		{"pause", "--note", "x"},
		{"pause", "--cabal", cabal.String(), "--all", "--note", "x"},
		{"pause", "--cabal", "not-a-uuid", "--note", "x"},
		{"pause", "--cabal", cabal.String()},
		{"pause", "--all", "--note", " "},
		{"resume"},
		{"resume", "--all", "extra"},
		{"resume", "--bogus"},
	} {
		if code, _, stderr := runOpsTool(t, ops, args...); code != 2 || !strings.Contains(stderr, opsUsage) {
			t.Errorf("ops %v = %d, stderr %q, want 2 and the usage", args, code, stderr)
		}
	}
}

func TestOps_pausesAndResumesACabalAsSystem(t *testing.T) {
	t.Parallel()
	ops, cabal, pool := seededOps(t)
	code, stdout, stderr := runOpsTool(t, ops, "pause", "--cabal", cabal.String(), "--note", "test")
	if code != 0 || !strings.HasPrefix(stdout, "paused "+cabal.String()+"\tpause_id=") {
		t.Fatalf("pause = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	code, stdout, stderr = runOpsTool(t, ops, "resume", "--cabal", cabal.String())
	if code != 0 || stdout != "resumed "+cabal.String()+"\n" {
		t.Fatalf("resume = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	var actors []string
	rows, err := pool.Query(t.Context(), `SELECT type || '=' || actor_type || ':' || actor_id FROM events
		WHERE type IN ('cabal.paused', 'cabal.resumed') ORDER BY created_at`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			t.Fatal(err)
		}
		actors = append(actors, a)
	}
	want := "cabal.paused=system:monacoctl,cabal.resumed=system:monacoctl"
	if got := strings.Join(actors, ","); got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
}

func TestOps_pausesAndResumesEveryCabal(t *testing.T) {
	t.Parallel()
	ops, _, _ := seededOps(t)
	if code, stdout, stderr := runOpsTool(t, ops, "pause", "--all", "--note", "incident"); code != 0 ||
		!strings.HasPrefix(stdout, "paused all\tpause_id=") {
		t.Fatalf("pause --all = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if code, stdout, stderr := runOpsTool(t, ops, "resume", "--all"); code != 0 || stdout != "resumed all\n" {
		t.Fatalf("resume --all = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestOps_reportsACabalStillPausedForAnExternalDeposit(t *testing.T) {
	t.Parallel()
	ops, cabal, pool := seededOps(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO cabal_pauses (id, cabal_id, reason, created_at)
		VALUES ($1, $2, 'external_deposit', now())`, ids.Real{}.NewV7(), cabal.UUID()); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runOpsTool(t, ops, "resume", "--cabal", cabal.String()); code != 1 ||
		!strings.Contains(stderr, "cabal_still_paused") {
		t.Fatalf("resume = %d, stderr %q, want 1 and cabal_still_paused", code, stderr)
	}
	unknown := ids.Real{}.NewV7().String()
	if code, _, stderr := runOpsTool(t, ops, "pause", "--cabal", unknown, "--note", "x"); code != 1 {
		t.Fatalf("pause of an unknown cabal = %d, stderr %q, want 1", code, stderr)
	}
}

func TestOps_reportsAnUnreachableDatabase(t *testing.T) {
	t.Parallel()
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://127.0.0.1:1/monaco?connect_timeout=1", "NATS_URL=nats://unused",
	}
	var stdout, stderr bytes.Buffer
	if code := toolOps(toolEnv{environ: environ})([]string{"resume", "--all"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "monacoctl: db.Open: ") {
		t.Fatalf("resume with no database = %d, stderr %q, want 1 and db.Open", code, stderr.String())
	}
}
