package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seededRoutedSwap(t *testing.T, pool *pgxpool.Pool, identifiers *testkit.IDs, at time.Time, park bool) uuid.UUID {
	t.Helper()
	id := identifiers.NewV7()
	q := sqlc.New(pool)
	if err := q.InsertCreated(t.Context(), sqlc.InsertCreatedParams{
		ID: id, SourceKind: "proposal", SourceID: identifiers.NewV7(), CabalID: identifiers.NewV7(),
		TreasuryAddress: "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin", Action: "buy", Symbol: "AAPLx",
		InMint: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", OutMint: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp",
		OutDecimals: 8, InAmount: 25_000_000, SlippageBps: 100, SourceBatchSize: 1, CreatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO trade_steps (id, trade_id, step, network, privy_action_id,
		amount_micros, status, created_at, updated_at) VALUES ($1, $2, 'bridge_out', 'base', $3, 25000000,
		'succeeded', $4, $4)`, identifiers.NewV7(), id, "privy-"+id.String(), at); err != nil {
		t.Fatal(err)
	}
	if park {
		if _, err := q.ParkRouted(
			t.Context(),
			sqlc.ParkRoutedParams{ID: id, At: at, Code: "asset_not_found"},
		); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func parkedSwapsTool(t *testing.T, input string) (tool, *pgxpool.Pool, *testkit.IDs, time.Time) {
	t.Helper()
	pool := testkit.DB(t)
	now := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=" + testkit.NATSURL(), "USER=alice",
	}
	return swapsTool(environ, testkit.NewClock(now), strings.NewReader(input)), pool, testkit.NewIDs(1), now
}

func runSwaps(t *testing.T, swaps tool, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := swaps(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestSwapsParked_listsAParkedSwapWithItsStepsAndOmitsAnUnparkedOne(t *testing.T) {
	t.Parallel()
	swaps, pool, identifiers, now := parkedSwapsTool(t, "")
	if code, out, _ := runSwaps(t, swaps, "parked"); code != 0 || out != "no parked swaps\n" {
		t.Fatalf("empty parked = %d, %q", code, out)
	}
	parked := seededRoutedSwap(t, pool, identifiers, now, true)
	seededRoutedSwap(t, pool, identifiers, now, false)
	var cabal uuid.UUID
	if err := pool.QueryRow(t.Context(), `SELECT cabal_id FROM swaps WHERE id = $1`, parked).Scan(&cabal); err != nil {
		t.Fatal(err)
	}

	code, out, stderr := runSwaps(t, swaps, "parked")

	want := "swap=" + parked.String() + " cabal=" + cabal.String() + " status=created parked_at=" +
		now.Format(time.RFC3339) + " code=asset_not_found bridge_out=succeeded swap_step= bridge_back=\n"
	if code != 0 || out != want {
		t.Fatalf("parked = %d, stdout %q, stderr %q, want %q", code, out, stderr, want)
	}
}

func wantContains(t *testing.T, label, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Fatalf("%s = %q, want it to contain %q", label, got, w)
		}
	}
}

func TestSwapsUnpark_clearsTheParkAndRecordsTheOperator(t *testing.T) {
	t.Parallel()
	swaps, pool, identifiers, now := parkedSwapsTool(t, "")
	swap := seededRoutedSwap(t, pool, identifiers, now, true)
	args := []string{"unpark", "--swap", swap.String(), "--reason", "rpc blip", "--yes"}

	code, out, stderr := runSwaps(t, swaps, args...)

	if code != 0 {
		t.Fatalf("unpark = %d, stderr %q", code, stderr)
	}
	wantContains(t, "unpark stdout", out, "before swap="+swap.String(), "code=asset_not_found",
		"after swap="+swap.String()+" ", "parked_at= code= ")
	if code, out, _ := runSwaps(t, swaps, "parked"); code != 0 || out != "no parked swaps\n" {
		t.Fatalf("parked after unpark = %d, %q", code, out)
	}
	var action string
	if err := pool.QueryRow(t.Context(), `SELECT actor || ' ' || reason || ' ' || (detail->>'parked_code')
		FROM swap_operator_actions WHERE swap_id = $1 AND action = 'unpark'`, swap).Scan(&action); err != nil ||
		action != "operator:alice rpc blip asset_not_found" {
		t.Fatalf("operator action = %q, %v", action, err)
	}
	code, _, stderr = runSwaps(t, swaps, args...)
	if code != 1 {
		t.Fatalf("second unpark = %d, want 1", code)
	}
	wantContains(t, "second unpark stderr", stderr, "swap_not_stuck")
}

func TestSwapsFailParked_failsTheSwapAfterTheOperatorConfirms(t *testing.T) {
	t.Parallel()
	swaps, pool, identifiers, now := parkedSwapsTool(t, "y\n")
	swap := seededRoutedSwap(t, pool, identifiers, now, true)

	code, out, stderr := runSwaps(t, swaps, "fail-parked", "--swap", swap.String(), "--reason", "listing gone")

	if code != 0 || !strings.Contains(stderr, "[y/N]") || !strings.Contains(out, "after swap="+swap.String()) ||
		!strings.Contains(out, "status=failed") {
		t.Fatalf("fail-parked = %d, stdout %q, stderr %q", code, out, stderr)
	}
	var status, failure string
	if err := pool.QueryRow(t.Context(), `SELECT status, failure_code FROM swaps WHERE id = $1`, swap).
		Scan(&status, &failure); err != nil || status != "failed" || failure != "never_submitted" {
		t.Fatalf("swap = %s %s, %v, want failed never_submitted", status, failure, err)
	}
}

func TestSwapsParked_rejectsMalformedArgumentsWithTheUsage(t *testing.T) {
	t.Parallel()
	swaps, _, _, _ := parkedSwapsTool(t, "")
	for _, args := range [][]string{
		{"parked", "extra"},
		{"unpark", "--reason", "rpc blip"},
		{"unpark", "--swap", "not-a-uuid", "--reason", "rpc blip"},
		{"fail-parked", "--swap", uuid.NewString()},
		{"fail-parked", "--swap", uuid.NewString(), "--reason", "x", "extra"},
	} {
		if code, _, stderr := runSwaps(t, swaps, args...); code != 2 || !strings.Contains(stderr, "swaps fail-parked") {
			t.Errorf("%v = %d, stderr %q, want the usage and exit 2", args, code, stderr)
		}
	}
}

func TestSwapsParked_reportsAnUnknownSwapADeclinedPromptAndAMissingDatabase(t *testing.T) {
	t.Parallel()
	swaps, pool, identifiers, now := parkedSwapsTool(t, "n\n")
	swap := seededRoutedSwap(t, pool, identifiers, now, true)

	code, _, stderr := runSwaps(t, swaps, "unpark", "--swap", uuid.NewString(), "--reason", "rpc blip", "--yes")
	if code != 1 {
		t.Fatalf("unpark of an unknown swap = %d, want 1", code)
	}
	wantContains(t, "unknown swap stderr", stderr, "swap_not_found")

	code, _, stderr = runSwaps(t, swaps, "fail-parked", "--swap", swap.String(), "--reason", "listing gone")
	if code != 1 {
		t.Fatalf("declined fail-parked = %d, want 1", code)
	}
	wantContains(t, "declined stderr", stderr, "[y/N]", "cancelled")
	var status string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM swaps WHERE id = $1`, swap).Scan(&status); err != nil ||
		status != "created" {
		t.Fatalf("swap after the declined prompt = %s, %v, want created", status, err)
	}

	missing := strings.Replace(pool.Config().ConnString(), "/"+pool.Config().ConnConfig.Database,
		"/missing_"+identifiers.NewV7().String()[:8], 1)
	gone := swapsTool([]string{"MONACO_ENV=test", "DATABASE_URL=" + missing, "NATS_URL=" + testkit.NATSURL()},
		testkit.NewClock(now), strings.NewReader(""))
	if code, _, stderr := runSwaps(t, gone, "parked"); code != 1 || stderr == "" {
		t.Fatalf("parked on a missing database = %d, stderr %q, want 1", code, stderr)
	}
}
