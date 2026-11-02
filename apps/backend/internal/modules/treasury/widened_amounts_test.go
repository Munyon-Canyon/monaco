package treasury_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestWidenedAmounts_aTwentyFiveTokenHoldingAt18DecimalsKeepsTheLedgerBalancedAndDriftFree(t *testing.T) {
	t.Parallel()
	s := seed(t)
	const (
		asset  = "0x1111111111111111111111111111111111111111"
		amount = "25000000000000000000"
	)
	s.exec(t, `WITH h AS (
			INSERT INTO cabal_txns (id, cabal_id, seq, kind, status, created_at)
			VALUES (gen_random_uuid(), $1, 99, 'swap', 'settled', now()) RETURNING id)
		INSERT INTO cabal_txn_entries (txn_id, seq, account, asset, amount)
		SELECT id, 0, 'treasury', $2, $3::numeric FROM h
		UNION ALL SELECT id, 1, 'venue', $2, -$3::numeric FROM h`, s.cabal.UUID(), asset, amount)
	s.exec(t, `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
		VALUES ($1, $2, $3::numeric, 0, now())`, s.cabal.UUID(), asset, amount)

	check := treasury.LedgerCheck(testkit.Config())
	if diffs, err := check.Check(t.Context(), s.f.pool); err != nil || len(diffs) != 0 {
		t.Fatalf("ledger with a 25-token holding = %q, %v, want no diffs", diffs, err)
	}

	s.exec(t, `UPDATE cabal_positions SET units = units + 1 WHERE asset = $1`, asset)
	diffs, err := check.Check(t.Context(), s.f.pool)
	want := "cabal_positions " + s.cabal.String() + " " + asset + ": entries " + amount + ", position 25000000000000000001"
	if err != nil || len(diffs) != 1 || diffs[0] != want {
		t.Fatalf("drifted ledger = %q, %v, want %q", diffs, err, want)
	}
}
