package ranking_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (r *valuationRig) reserve(t *testing.T, payout uint64) {
	t.Helper()
	_, err := r.pool.Exec(t.Context(), `INSERT INTO cash_out_jobs
		(id, cabal_id, user_id, share_units, payout_micros, slice_micros, status, created_at, updated_at)
		VALUES ($1, $2, $3, 1, $4, $4, 'selling', $5, $5)`,
		ids.Real{}.NewV7(), r.cabal.ID.UUID(), r.cabal.Creator.ID.UUID(), payout, r.now)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunValuation_navEqualsThePotValueTreasuryReports(t *testing.T) {
	t.Parallel()
	rig := newValuationRig(t)
	at := rig.now.Add(time.Minute)
	for _, tc := range []struct{ reserved, pot uint64 }{{0, 106_000_000}, {30_000_000, 76_000_000}} {
		if tc.reserved != 0 {
			rig.reserve(t, tc.reserved)
		}
		pot, err := rig.treasury.PotValue(t.Context(), rig.cabal.ID)
		if err != nil {
			t.Fatal(err)
		}
		got := rig.run(t, at)
		if len(got.Cabals) != 1 || got.Cabals[0].Value != pot {
			t.Fatalf("with %d reserved: NAV = %+v, PotValue = %v, want them equal", tc.reserved, got.Cabals, pot)
		}
		if want := money.MicrosFromUint64(tc.pot); pot != want {
			t.Fatalf(
				"with %d reserved: PotValue = %v, want %v (100 USDC plus 3 shares at 2 USDC)",
				tc.reserved,
				pot,
				want,
			)
		}
	}
}
