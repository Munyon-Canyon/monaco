package trading_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const insertLedgerSwap = `INSERT INTO swaps (id, source_kind, source_id, cabal_id, treasury_address, action, symbol, in_mint,
  out_mint, in_amount, slippage_bps, status, created_at, confirmed_at, updated_at)
VALUES (gen_random_uuid(), 'proposal', gen_random_uuid(), $1, 'treasury', $2, $3, 'in', 'out', 1, 50, $4, $5, $6, $5)`

func TestFilledBuysBetween_LedgerBuys(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cabal := ids.CabalIDFrom(uuid.NewSHA1(uuid.Nil, []byte("cabal")))
	other := ids.CabalIDFrom(uuid.NewSHA1(uuid.Nil, []byte("other")))
	start := time.Date(2026, 10, 11, 17, 0, 0, 0, time.UTC)
	end := start.Add(7 * 24 * time.Hour)
	for _, s := range []struct {
		cabal  ids.CabalID
		action string
		symbol string
		status string
		at     time.Time
	}{
		{cabal, "buy", "NVDA", "confirmed", start.Add(time.Hour)},
		{cabal, "sell", "AMZN", "confirmed", start.Add(2 * time.Hour)},
		{cabal, "buy", "TSLA", "failed", start.Add(3 * time.Hour)},
		{cabal, "buy", "AAPL", "confirmed", start.Add(-time.Second)},
		{cabal, "buy", "MSFT", "confirmed", end},
		{other, "buy", "META", "confirmed", start.Add(4 * time.Hour)},
	} {
		var confirmed any
		if s.status == "confirmed" {
			confirmed = s.at
		}
		if _, err := pool.Exec(
			t.Context(),
			insertLedgerSwap,
			s.cabal.UUID(),
			s.action,
			s.symbol,
			s.status,
			s.at,
			confirmed,
		); err != nil {
			t.Fatal(err)
		}
	}
	got, err := app.NewQueries(pool).FilledBuysBetween(t.Context(), []ids.CabalID{cabal}, start, end)
	if err != nil || len(got) != 1 || got[0].Symbol != "NVDA" || got[0].CabalID != cabal ||
		!got[0].ConfirmedAt.Equal(start.Add(time.Hour)) {
		t.Fatalf("FilledBuysBetween = %+v, %v; want only the confirmed NVDA buy in the window", got, err)
	}
}

func TestFilledBuysBetween_DatabaseErrorIsInternal(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	pool.Close()
	if _, err := app.NewQueries(pool).FilledBuysBetween(t.Context(), nil, time.Time{}, time.Time{}); err == nil {
		t.Fatal("FilledBuysBetween on a closed pool must fail")
	}
}
