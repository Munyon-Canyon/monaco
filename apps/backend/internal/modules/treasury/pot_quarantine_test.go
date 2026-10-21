package treasury_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestPotValue_UnsolicitedDepositsNeverReachNAVOrShares(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), testkit.NewCabal(t, f.pool).ID
	u, c, err := f.fund(user, cabal, 100_000_000, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	q := newQueries(f)
	before, err := q.PotValue(t.Context(), cabal)
	if err != nil || before != money.MicrosFromUint64(100_000_000) {
		t.Fatalf("PotValue = %v, %v, want the funded 100 USDC", before, err)
	}
	shares := f.positionUnits(t, cabal, user)

	for i, status := range []string{"detected", "bouncing", "bounce_failed", "held"} {
		if _, err := f.pool.Exec(t.Context(), `INSERT INTO external_deposits
			(id, signature, cabal_id, sender, mint, amount, source, status, detected_at)
			VALUES ($1, $2, $3, 'sender', $4, 25000000, 'reconcile', $5, now())`,
			f.ids.NewV7(), "stray-"+status, cabal.UUID(), usdcMint, status); err != nil {
			t.Fatalf("stray deposit %d: %v", i, err)
		}
		after, err := q.PotValue(t.Context(), cabal)
		if err != nil || after != before {
			t.Fatalf("PotValue = %v, %v after a %s stray deposit, want %v", after, err, status, before)
		}
	}
	if got := f.positionUnits(t, cabal, user); got != shares {
		t.Fatalf("shares = %d after the stray deposits, want %d", got, shares)
	}
	if drift := f.drift(t); len(drift) != 0 {
		t.Fatalf("ledger drift = %#v", drift)
	}
}
