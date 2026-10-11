package main

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seededFailedBridgeBack(
	t *testing.T,
	pool *pgxpool.Pool,
	identifiers *testkit.IDs,
	at time.Time,
) (uuid.UUID, uuid.UUID) {
	t.Helper()
	swap := seededRoutedSwap(t, pool, identifiers, at, false)
	var cabal uuid.UUID
	if err := pool.QueryRow(t.Context(), `INSERT INTO trade_steps (id, trade_id, step, network, privy_action_id,
		amount_micros, status, error, created_at, updated_at) VALUES ($1, $2, 'bridge_back', 'base', $3, 24990000,
		'failed', 'privy_rejected', $4, $4) RETURNING (SELECT cabal_id FROM swaps WHERE id = $2)`,
		identifiers.NewV7(), swap, "privy-back-"+swap.String(), at).Scan(&cabal); err != nil {
		t.Fatal(err)
	}
	return swap, cabal
}

func TestSwapsFailedBridgeBacks_listsEachFailedReturn(t *testing.T) {
	t.Parallel()
	swaps, pool, identifiers, now := parkedSwapsTool(t, "")
	if code, out, _ := runSwaps(t, swaps, "failed-bridge-backs"); code != 0 || out != "no failed bridge-backs\n" {
		t.Fatalf("empty failed-bridge-backs = %d, %q", code, out)
	}
	swap, cabal := seededFailedBridgeBack(t, pool, identifiers, now)

	code, out, stderr := runSwaps(t, swaps, "failed-bridge-backs")

	want := "swap=" + swap.String() + " cabal=" + cabal.String() + " source=proposal network=base " +
		"amount_micros=24990000 attempt=1 error=privy_rejected failed_at=" + now.Format(time.RFC3339) + "\n"
	if code != 0 || out != want {
		t.Fatalf("failed-bridge-backs = %d, stdout %q, stderr %q, want %q", code, out, stderr, want)
	}
}

func TestSwapsRetryBridgeBack_queuesTheNextAttemptAndRecordsTheOperator(t *testing.T) {
	t.Parallel()
	swaps, pool, identifiers, now := parkedSwapsTool(t, "y\n")
	swap, _ := seededFailedBridgeBack(t, pool, identifiers, now)

	code, out, stderr := runSwaps(t, swaps, "retry-bridge-back", "--swap", swap.String(), "--reason",
		"privy rejected, balance restored")

	if code != 0 {
		t.Fatalf("retry-bridge-back = %d, stderr %q", code, stderr)
	}
	wantContains(t, "retry stderr", stderr, "[y/N]")
	wantContains(
		t,
		"retry stdout",
		out,
		"before swap="+swap.String(),
		"status=failed attempt=1 privy_action_id=privy-back-",
		"after swap="+swap.String(),
		"status=pending attempt=2 privy_action_id= prior_privy_action_ids=privy-back-"+
			swap.String()+" error=\n",
	)
	var action string
	if err := pool.QueryRow(t.Context(), `SELECT actor || ' ' || reason || ' ' || (detail->>'attempt') || ' ' ||
		(detail->>'prior_privy_action_id') || ' ' || (detail->>'prior_error')
		FROM swap_operator_actions WHERE swap_id = $1 AND action = 'retry_bridge_back'`, swap).Scan(&action); err != nil ||
		action != "operator:alice privy rejected, balance restored 2 privy-back-"+swap.String()+" privy_rejected" {
		t.Fatalf("operator action = %q, %v", action, err)
	}
	if code, out, _ := runSwaps(t, swaps, "failed-bridge-backs"); code != 0 || out != "no failed bridge-backs\n" {
		t.Fatalf("failed-bridge-backs after the retry = %d, %q", code, out)
	}
	code, _, stderr = runSwaps(t, swaps, "retry-bridge-back", "--swap", swap.String(), "--reason", "again", "--yes")
	if code != 1 {
		t.Fatalf("second retry = %d, want 1", code)
	}
	wantContains(t, "second retry stderr", stderr, "swap_not_stuck")
}

func TestSwapsRetryBridgeBack_refusesBadArgumentsAnUnknownSwapAndADeclinedPrompt(t *testing.T) {
	t.Parallel()
	swaps, pool, identifiers, now := parkedSwapsTool(t, "n\n")
	swap, _ := seededFailedBridgeBack(t, pool, identifiers, now)
	for _, args := range [][]string{
		{"failed-bridge-backs", "extra"},
		{"retry-bridge-back", "--swap", swap.String()},
		{"retry-bridge-back", "--swap", "not-a-uuid", "--reason", "x"},
		{"retry-bridge-back", "--swap", swap.String(), "--reason", "x", "extra"},
	} {
		if code, _, stderr := runSwaps(t, swaps, args...); code != 2 || !strings.Contains(stderr, "retry-bridge-back") {
			t.Errorf("%v = %d, stderr %q, want the usage and exit 2", args, code, stderr)
		}
	}
	code, _, stderr := runSwaps(t, swaps, "retry-bridge-back", "--swap", uuid.NewString(), "--reason", "x", "--yes")
	if code != 1 {
		t.Fatalf("retry of an unknown swap = %d, want 1", code)
	}
	wantContains(t, "unknown swap stderr", stderr, "swap_not_found")

	code, _, stderr = runSwaps(t, swaps, "retry-bridge-back", "--swap", swap.String(), "--reason", "x")
	if code != 1 {
		t.Fatalf("declined retry = %d, want 1", code)
	}
	wantContains(t, "declined stderr", stderr, "cancelled")
	var status string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM trade_steps WHERE trade_id = $1 AND step = 'bridge_back'`,
		swap).Scan(&status); err != nil || status != "failed" {
		t.Fatalf("bridge_back after the declined prompt = %s, %v, want failed", status, err)
	}
}

func TestSwapsFailedBridgeBacks_reportsAMissingDatabase(t *testing.T) {
	t.Parallel()
	_, pool, identifiers, now := parkedSwapsTool(t, "")
	missing := strings.Replace(pool.Config().ConnString(), "/"+pool.Config().ConnConfig.Database,
		"/missing_"+identifiers.NewV7().String()[:8], 1)
	gone := swapsTool([]string{"MONACO_ENV=test", "DATABASE_URL=" + missing, "NATS_URL=" + testkit.NATSURL()},
		testkit.NewClock(now), strings.NewReader(""))

	if code, _, stderr := runSwaps(t, gone, "failed-bridge-backs"); code != 1 || stderr == "" {
		t.Fatalf("failed-bridge-backs on a missing database = %d, stderr %q, want 1", code, stderr)
	}
}
