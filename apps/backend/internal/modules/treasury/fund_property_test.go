package treasury_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestFundProperty_headersShareSettledAndTheLedgerReplays(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	h.stubs.onChain = money.MicrosFromUint64(1_000_000_000_000)
	rapid.Check(t, func(rt *rapid.T) {
		h.user, h.cabal = h.fixture.user(t), h.fixture.cabal(t)
		for range rapid.IntRange(1, 3).Draw(rt, "funds") {
			micros := rapid.Uint64Range(1_000_000, 500_000_000).Draw(rt, "micros")
			_, sig := h.submitted(t, micros)
			h.finalize(sig)
			if rapid.Bool().Draw(rt, "pot gains") {
				h.gain(rt, rapid.Int64Range(1, 100_000_000).Draw(rt, "gain"))
			}
			for range rapid.IntRange(1, 2).Draw(rt, "ticks") {
				h.tick(t)
			}
			h.redeliverFunded(t)
		}
		h.checkFundInvariants(rt)
	})
}

func (h *settleHarness) gain(rt *rapid.T, micros int64) {
	c, err := domain.NewCabalTxn(domain.CabalTxnHeader{
		ID: h.ids.NewV7(), CabalID: h.cabal, Kind: domain.CabalSwap, Status: domain.TxnSettled, SwapID: h.ids.NewV7(),
	}, []domain.CabalEntry{
		{Account: domain.CabalTreasury, Asset: h.usdc(), Amount: amount(micros)},
		{Account: domain.CabalVenue, Asset: h.usdc(), Amount: amount(-micros)},
	})
	if err == nil {
		err = h.postCabal(c)
	}
	if err != nil {
		rt.Fatal(err)
	}
}

func (h *settleHarness) redeliverFunded(t *testing.T) {
	t.Helper()
	rows, err := h.pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1`, string(events.TypeFunded))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var e events.Funded
		if err := rows.Scan(&e); err != nil {
			t.Fatal(err)
		}
		if err := h.deliver(t, e, submittedAt()); err != nil {
			t.Fatal(err)
		}
	}
}

func (h *settleHarness) checkFundInvariants(rt *rapid.T) {
	if drift := h.drift(h.t); len(drift) != 0 {
		rt.Fatalf("ledger drift = %v", drift)
	}
	var split, unsettled, minted, funded int
	if err := h.pool.QueryRow(h.t.Context(), `SELECT
		(SELECT count(*) FROM (SELECT transfer_id FROM (SELECT transfer_id FROM user_txns WHERE kind = 'fund'
			UNION ALL SELECT transfer_id FROM cabal_txns WHERE kind = 'fund') h GROUP BY 1 HAVING count(*) <> 2) s),
		(SELECT count(*) FROM fund_transfers WHERE status <> 'settled') +
			(SELECT count(*) FROM user_txns WHERE kind = 'fund' AND status <> 'settled') +
			(SELECT count(*) FROM cabal_txns WHERE kind = 'fund' AND status <> 'settled'),
		(SELECT count(*) FROM fund_transfers),
		(SELECT count(*) FROM events WHERE type = 'cabal.funded')`).Scan(&split, &unsettled, &minted, &funded); err != nil {
		rt.Fatal(err)
	}
	if split != 0 || unsettled != 0 || funded != minted {
		rt.Fatalf("transfers split across headers %d, unsettled %d, funded events %d for %d transfers",
			split, unsettled, funded, minted)
	}
}
