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

func seededBounce(t *testing.T) (tool, string, *pgxpool.Pool) {
	t.Helper()
	pool := testkit.DB(t)
	cabal := testkit.NewCabal(t, pool)
	deposit := ids.Real{}.NewV7()
	if _, err := pool.Exec(t.Context(), `INSERT INTO external_deposits (id, signature, cabal_id, sender, mint, amount,
		source, status, detected_at) VALUES ($1, 'stray', $2, 'sender', 'mint', 25000000, 'reconcile', 'detected', now())`,
		deposit, cabal.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO cabal_pauses (id, cabal_id, reason, created_at, external_deposit_id)
		VALUES ($1, $2, 'external_deposit', now(), $3)`,
		ids.Real{}.NewV7(),
		cabal.ID.UUID(),
		deposit,
	); err != nil {
		t.Fatal(err)
	}
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://unused", "USER=ops-person",
	}
	return fundingTool(environ, testkit.NewClock(clock.Real{}.Now().UTC())), deposit.String(), pool
}

func TestFundingBounce_printsTheUsageForHelpAndMalformedArguments(t *testing.T) {
	t.Parallel()
	bounce, id, _ := seededBounce(t)
	if code, stdout, _ := runOpsTool(t, bounce, "bounce", "hold", "--help"); code != 0 ||
		!strings.Contains(stdout, fundingUsage) {
		t.Fatalf("hold --help = %d, stdout %q, want 0 and the usage", code, stdout)
	}
	for _, args := range [][]string{
		{"bounce"},
		{"bounce", "hold"},
		{"bounce", "hold", "not-a-uuid"},
		{"bounce", "set-return-address", id},
		{"bounce", "bogus", id},
		{"bounce", "retry", id, "extra"},
	} {
		if code, _, stderr := runOpsTool(t, bounce, args...); code != 2 || !strings.Contains(stderr, fundingUsage) {
			t.Errorf("funding %v = %d, stderr %q, want 2 and the usage", args, code, stderr)
		}
	}
}

func TestFundingBounce_holdsADetectedDepositAndEndsItsPause(t *testing.T) {
	t.Parallel()
	bounce, id, pool := seededBounce(t)
	if code, stdout, stderr := runOpsTool(t, bounce, "bounce", "hold", id); code != 0 || stdout != "hold "+id+"\n" {
		t.Fatalf("hold = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	var status string
	var open int
	if err := pool.QueryRow(t.Context(), `SELECT status, (SELECT count(*) FROM cabal_pauses WHERE resolved_at IS NULL)
		FROM external_deposits`).Scan(&status, &open); err != nil || status != "held" || open != 0 {
		t.Fatalf("status = %s with %d open pauses, %v, want held and none", status, open, err)
	}
	if code, _, stderr := runOpsTool(t, bounce, "bounce", "hold", id); code != 1 ||
		!strings.Contains(stderr, "version_conflict") {
		t.Fatalf("second hold = %d, stderr %q, want 1 and version_conflict", code, stderr)
	}
}

func TestFundingBounce_setsAReturnAddress(t *testing.T) {
	t.Parallel()
	bounce, id, pool := seededBounce(t)
	to := "9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P"
	if code, _, stderr := runOpsTool(t, bounce, "bounce", "set-return-address", id, to); code != 0 {
		t.Fatalf("set-return-address = %d, stderr %q", code, stderr)
	}
	var got string
	if err := pool.QueryRow(t.Context(), `SELECT return_address FROM external_deposits`).Scan(&got); err != nil ||
		got != to {
		t.Fatalf("return_address = %q, %v, want %s", got, err, to)
	}
	if code, _, _ := runOpsTool(t, bounce, "bounce", "retry", id); code != 1 {
		t.Fatalf("retry of a detected deposit = %d, want 1", code)
	}
}

func TestFundingBounce_namesTheOperator(t *testing.T) {
	t.Parallel()
	if got := operatorOf([]string{"HOME=/x", "USER=ops-person"}); got != "ops-person" {
		t.Fatalf("operator = %q, want ops-person", got)
	}
	if got := operatorOf([]string{"USER="}); got != "unknown" {
		t.Fatalf("operator = %q, want unknown", got)
	}
}

func TestFundingBounce_reportsAnUnreachableDatabase(t *testing.T) {
	t.Parallel()
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://127.0.0.1:1/monaco?connect_timeout=1", "NATS_URL=nats://unused",
	}
	var stdout, stderr bytes.Buffer
	args := []string{"bounce", "hold", ids.Real{}.NewV7().String()}
	if code := toolFunding(toolEnv{environ: environ})(args, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "monacoctl: db.Open: ") {
		t.Fatalf("hold with no database = %d, stderr %q, want 1 and db.Open", code, stderr.String())
	}
}

func TestFundingMigrateCursors_copiesEachCursorOnceAndIsIdempotent(t *testing.T) {
	t.Parallel()
	bounce, _, pool := seededBounce(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO deposit_cursors (wallet_address, last_signature, cursor_slot, scanned_at) VALUES ($1, 'sig', 7, now())`,
		user.Address,
	); err != nil {
		t.Fatal(err)
	}
	for run, want := range []string{"migrated 1 wallets\n", "migrated 0 wallets\n"} {
		if code, stdout, stderr := runOpsTool(t, bounce, "migrate-cursors"); code != 0 || stdout != want {
			t.Fatalf("run %d = %d, stdout %q, stderr %q; want %q", run+1, code, stdout, stderr, want)
		}
	}
	assertMigratedCursor(t, pool)
}

func assertMigratedCursor(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var wallets, accounts int
	var high string
	var slot, first, dirty, clean int64
	if err := pool.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM deposit_watch_wallets), count(*), min(a.high_signature), min(a.high_slot),
		(SELECT min(first_seen_slot) FROM deposit_watch_wallets), min(a.dirty_gen), min(a.clean_gen)
		FROM deposit_watch_accounts a WHERE a.canonical`).Scan(&wallets, &accounts, &high, &slot, &first, &dirty, &clean); err != nil {
		t.Fatal(err)
	}
	if wallets != 1 || accounts != 1 || high != "sig" || slot != 7 || first != 7 || dirty != 1 || clean != 0 {
		t.Fatalf("wallets/accounts/high/slot/first/dirty/clean = %d/%d/%q/%d/%d/%d/%d, want 1/1/sig/7/7/1/0",
			wallets, accounts, high, slot, first, dirty, clean)
	}
}

func TestFundingMigrateCursors_rejectsArgumentsAndReportsFailures(t *testing.T) {
	t.Parallel()
	bounce, _, pool := seededBounce(t)
	if code, _, stderr := runOpsTool(
		t,
		bounce,
		"migrate-cursors",
		"extra",
	); code != 2 ||
		!strings.Contains(stderr, fundingUsage) {
		t.Fatalf("migrate-cursors extra = %d, stderr %q, want 2 and the usage", code, stderr)
	}
	if _, err := pool.Exec(t.Context(), `DROP TABLE deposit_cursors`); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runOpsTool(t, bounce, "migrate-cursors"); code != 1 || stderr == "" {
		t.Fatalf("migrate-cursors without the cursor table = %d, stderr %q, want 1", code, stderr)
	}
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://127.0.0.1:1/monaco?connect_timeout=1", "NATS_URL=nats://unused",
	}
	var stdout, stderr bytes.Buffer
	if code := toolFunding(toolEnv{environ: environ})([]string{"migrate-cursors"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "monacoctl: db.Open: ") {
		t.Fatalf("migrate-cursors with no database = %d, stderr %q, want 1 and db.Open", code, stderr.String())
	}
}
