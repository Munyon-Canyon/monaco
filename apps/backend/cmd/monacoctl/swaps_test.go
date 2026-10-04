package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seededSubmittedSwap(t *testing.T, input string) (tool, string, uuid.UUID, *pgxpool.Pool) {
	t.Helper()
	pool := testkit.DB(t)
	now := time.Now().UTC().Add(-time.Hour)
	identifiers := testkit.NewIDs(1)
	id := identifiers.NewV7()
	signature := "sig-" + id.String()
	q := sqlc.New(pool)
	if err := q.InsertCreated(t.Context(), sqlc.InsertCreatedParams{
		ID: id, SourceKind: "proposal", SourceID: identifiers.NewV7(), CabalID: identifiers.NewV7(),
		TreasuryAddress: "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin", Action: "buy", Symbol: "AAPLx",
		InMint: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", OutMint: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp",
		OutDecimals: 8, InAmount: 25_000_000, QuoteOutAmount: pgtype.Int8{Int64: 105_000_000, Valid: true},
		SlippageBps: 100, SourceBatchSize: 1, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.MarkSubmitted(t.Context(), sqlc.MarkSubmittedParams{
		ID: id, ExecuteRequestID: "request-" + id.String(), SignedTx: []byte{1}, TxSignature: signature,
		SubmittedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=" + testkit.NATSURL(),
	}
	return swapsTool(environ, testkit.NewClock(now), strings.NewReader(input)), signature, id, pool
}

func TestSwapsForceResolve_confirmsOnceAndPrintsBeforeAndAfter(t *testing.T) {
	t.Parallel()
	swaps, signature, id, pool := seededSubmittedSwap(t, "")
	args := []string{
		"force-resolve", "--signature", signature, "--to", "confirmed", "--out-amount", "104000000",
		"--reason", "chain explorer verified it", "--yes",
	}
	assertForceResolveOutput(t, swaps, args,
		[]string{"before swap=", "status=submitted", "after swap=", "status=confirmed"},
		[]string{"trading.swap.force_resolved", "chain explorer verified it"},
	)
	var stdout, stderr bytes.Buffer
	stdout.Reset()
	stderr.Reset()
	if code := swaps(args, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "status=confirmed") {
		t.Fatalf("second force-resolve = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	var events int
	const confirmedEvents = `SELECT count(*) FROM events WHERE aggregate_id = $1 AND type = 'trade.confirmed'`
	row := pool.QueryRow(t.Context(), confirmedEvents, id)
	if err := row.Scan(&events); err != nil || events != 1 {
		t.Fatalf("confirmed events = %d, %v, want 1", events, err)
	}
}

func TestSwapsForceResolve_refusesConflictingTerminalStatus(t *testing.T) {
	t.Parallel()
	swaps, signature, _, _ := seededSubmittedSwap(t, "")
	confirmed := []string{
		"force-resolve", "--signature", signature, "--to", "confirmed", "--out-amount", "104000000",
		"--reason", "chain explorer verified it", "--yes",
	}
	assertForceResolveSuccess(t, swaps, confirmed)
	failed := []string{
		"force-resolve", "--signature", signature, "--to", "failed", "--reason", "conflicting resolution", "--yes",
	}
	assertForceResolveFailure(t, swaps, failed, "swap_not_stuck")
}

func TestSwapsForceResolve_promptsBeforeFailing(t *testing.T) {
	t.Parallel()
	swaps, signature, id, pool := seededSubmittedSwap(t, "n\n")
	args := []string{
		"force-resolve", "--signature", signature, "--to", "failed", "--reason", "chain explorer verified it",
	}
	var stdout, stderr bytes.Buffer
	if code := swaps(args, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "[y/N]") ||
		!strings.Contains(stderr.String(), "cancelled") {
		t.Fatalf("cancelled force-resolve = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	var events int
	const allEvents = `SELECT count(*) FROM events WHERE aggregate_id = $1`
	if err := pool.QueryRow(t.Context(), allEvents, id).Scan(&events); err != nil || events != 0 {
		t.Fatalf("events after cancellation = %d, %v, want 0", events, err)
	}
	swaps = swapsTool(
		[]string{"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=" + testkit.NATSURL()},
		testkit.NewClock(time.Now().UTC()), strings.NewReader("y\n"),
	)
	stdout.Reset()
	stderr.Reset()
	if code := swaps(args, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "status=failed") {
		t.Fatalf("accepted force-resolve = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestSwapsForceResolve_rejectsMalformedArgumentsWithTheUsage(t *testing.T) {
	t.Parallel()
	swaps, signature, _, _ := seededSubmittedSwap(t, "")
	for _, args := range [][]string{
		{"force-resolve", "--to", "failed", "--reason", "verified"},
		{"force-resolve", "--signature", signature, "--to", "submitted", "--reason", "verified"},
		{"force-resolve", "--signature", signature, "--to", "confirmed", "--reason", "verified"},
		{"force-resolve", "--signature", signature, "--to", "failed", "--out-amount", "1", "--reason", "verified"},
		{
			"force-resolve", "--signature", signature, "--to", "confirmed", "--out-amount", "not-a-number",
			"--reason", "verified",
		},
	} {
		var stdout, stderr bytes.Buffer
		if code := swaps(args, &stdout, &stderr); code != 2 || stdout.Len() != 0 ||
			!strings.Contains(stderr.String(), swapsUsage) {
			t.Errorf(
				"swaps %v = %d, stdout %q, stderr %q, want 2 and the usage",
				args,
				code,
				stdout.String(),
				stderr.String(),
			)
		}
	}
}

func TestSwapsForceResolve_createdSwapReturnsNotStuck(t *testing.T) {
	t.Parallel()
	swaps, signature, id, pool := seededSubmittedSwap(t, "")
	if _, err := pool.Exec(t.Context(), `UPDATE swaps SET status = 'created' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	args := []string{"force-resolve", "--signature", signature, "--to", "failed", "--reason", "verified", "--yes"}
	assertForceResolveFailure(t, swaps, args, "swap_not_stuck")
}

func TestMatchesResolvedSwap(t *testing.T) {
	t.Parallel()
	confirmed := trading.SwapView{Status: "confirmed", OutAmount: 7}
	for _, test := range []struct {
		name string
		to   domain.Status
		out  uint64
		want bool
	}{
		{name: "matching confirmed fill", to: "confirmed", out: 7, want: true},
		{name: "different confirmed fill", to: "confirmed", out: 8},
		{name: "different terminal status", to: "failed", out: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := matchesResolvedSwap(confirmed, test.to, test.out); got != test.want {
				t.Fatalf("matchesResolvedSwap() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestForceResolveUnits_refusesInvalidDecimals(t *testing.T) {
	t.Parallel()
	for _, decimals := range []int16{-1, 256} {
		if _, err := forceResolveUnits(decimals, 1, "confirmed"); err == nil {
			t.Errorf("forceResolveUnits(%d) succeeded", decimals)
		}
	}
	if got, err := forceResolveUnits(8, 1, "failed"); err != nil || !got.IsZero() {
		t.Fatalf("failed units = %#v, %v", got, err)
	}
}

func TestSwapsForceResolve_reportsAnUnreachableDatabase(t *testing.T) {
	t.Parallel()
	environ := []string{
		"MONACO_ENV=test",
		"DATABASE_URL=postgres://127.0.0.1:1/monaco?connect_timeout=1",
		"NATS_URL=" + testkit.NATSURL(),
	}
	args := []string{
		"force-resolve", "--signature", ids.Real{}.NewV7().String(), "--to", "failed", "--reason", "verified", "--yes",
	}
	var stdout, stderr bytes.Buffer
	if code := swapsTool(environ, clock.Real{}, strings.NewReader(""))(args, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "monacoctl: db.Open: ") {
		t.Fatalf("force-resolve with no database = %d, stderr %q, want 1 and db.Open", code, stderr.String())
	}
}

func TestSwapsForceResolve_reportsOperationalFailures(t *testing.T) {
	t.Parallel()
	t.Run("missing swap", func(t *testing.T) {
		t.Parallel()
		swaps, _, _, _ := seededSubmittedSwap(t, "")
		args := []string{"force-resolve", "--signature", "missing", "--to", "failed", "--reason", "verified", "--yes"}
		assertForceResolveFailure(t, swaps, args, "swap_not_found")
	})
	t.Run("nats unavailable", func(t *testing.T) {
		t.Parallel()
		_, signature, _, pool := seededSubmittedSwap(t, "")
		swaps := swapsTool(
			[]string{"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://127.0.0.1:1"},
			clock.Real{}, strings.NewReader(""),
		)
		args := []string{"force-resolve", "--signature", signature, "--to", "failed", "--reason", "verified", "--yes"}
		assertForceResolveFailure(t, swaps, args, "upstream_unavailable")
	})
	t.Run("invalid stored decimals", func(t *testing.T) {
		t.Parallel()
		swaps, signature, id, pool := seededSubmittedSwap(t, "")
		if _, err := pool.Exec(t.Context(), `UPDATE swaps SET out_decimals = -1 WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		args := []string{
			"force-resolve", "--signature", signature, "--to", "confirmed", "--out-amount", "1", "--reason", "verified",
			"--yes",
		}
		assertForceResolveFailure(t, swaps, args, "decode_failed")
	})
	t.Run("invalid command", func(t *testing.T) {
		t.Parallel()
		swaps, signature, _, _ := seededSubmittedSwap(t, "")
		args := []string{
			"force-resolve", "--signature", signature, "--to", "failed", "--reason", strings.Repeat("a", 501), "--yes",
		}
		assertForceResolveFailure(t, swaps, args, "invalid_input")
	})
}

func assertForceResolveFailure(t *testing.T, swaps tool, args []string, match string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := swaps(args, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), match) {
		t.Fatalf(
			"force-resolve = %d, stdout %q, stderr %q, want 1 and %q",
			code,
			stdout.String(),
			stderr.String(),
			match,
		)
	}
}

func assertForceResolveSuccess(t *testing.T, swaps tool, args []string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := swaps(args, &stdout, &stderr); code != 0 {
		t.Fatalf("force-resolve = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func assertForceResolveOutput(t *testing.T, swaps tool, args, stdoutMatches, stderrMatches []string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := swaps(args, &stdout, &stderr); code != 0 {
		t.Fatalf("force-resolve = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	for _, match := range stdoutMatches {
		if !strings.Contains(stdout.String(), match) {
			t.Fatalf("stdout %q does not contain %q", stdout.String(), match)
		}
	}
	for _, match := range stderrMatches {
		if !strings.Contains(stderr.String(), match) {
			t.Fatalf("stderr %q does not contain %q", stderr.String(), match)
		}
	}
}
