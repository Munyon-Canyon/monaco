package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type roundTrip struct {
	f     fixture
	cabal ids.CabalID
	user  ids.UserID
	buy   uuid.UUID
	sell  uuid.UUID
	at    time.Time
}

func newRoundTrip(t *testing.T) roundTrip {
	t.Helper()
	f := newFixture(t)
	const micros = 200_000_000
	cabal := f.cabal(t)
	u, c, err := f.fund(f.user(t), cabal, micros, micros, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	f.bridgedOut(t, cabal, 110_000_000, 0)
	return roundTrip{
		f: f, cabal: cabal, user: f.user(t), buy: f.ids.NewV7(), sell: f.ids.NewV7(),
		at: clock.Real{}.Now().UTC().Truncate(time.Second).Add(-24 * time.Hour),
	}
}

func (r roundTrip) confirm(t *testing.T, e events.TradeConfirmed, at time.Time) {
	t.Helper()
	h := adapters.Activity{Hints: &hints{}}
	if err := r.f.do(func(ctx context.Context, tx db.Tx) error { return h.Confirmed(ctx, tx, e, at) }); err != nil {
		t.Fatal(err)
	}
}

func (r roundTrip) bought(t *testing.T, landed, fee uint64) {
	t.Helper()
	r.confirm(t, events.TradeConfirmed{
		V: 1, SwapID: r.buy, CabalID: r.cabal.UUID(), Action: "buy", Symbol: "AAPLx",
		InMint: baseCash, InAmount: landed, OutMint: perfEVM, OutAmount: 1_000_000,
		USDCMicros: money.MicrosFromUint64(landed + fee), FeeMicros: money.MicrosFromUint64(fee),
	}, r.at)
}

func (r roundTrip) sold(t *testing.T) {
	t.Helper()
	const gross = 110_000_000
	r.confirm(t, events.TradeConfirmed{
		V: 1, SwapID: r.sell, CabalID: r.cabal.UUID(), Action: "sell", Symbol: "AAPLx",
		InMint: perfEVM, InAmount: 1_000_000, OutMint: baseCash, OutAmount: gross,
		USDCMicros: money.MicrosFromUint64(gross),
	}, r.at.Add(time.Hour))
}

func (r roundTrip) bridgedBack(t *testing.T, sent, fee uint64) {
	t.Helper()
	back := bridgeRun{f: r.f, cabal: r.cabal, swap: r.sell, step: "bridge_back"}
	back.submitted(t, sent)
	back.succeeded(t, sent, fee)
}

func (r roundTrip) read(t *testing.T) app.HoldingPerformance {
	t.Helper()
	got, err := perfReads(r.f.pool, true).Handle(t.Context(),
		app.HoldingPerformanceQuery{CabalID: r.cabal, Caller: r.user, Symbol: "AAPLx"})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (r roundTrip) want(t *testing.T, avgSell uint64, realized int64) {
	t.Helper()
	got := r.read(t)
	if got.AvgSellMicros == nil || got.AvgSellMicros.Uint64() != avgSell || got.RealizedPnLMicros.Int64() != realized {
		t.Fatalf("performance = %v, %d, want avg sell %d and realized %d", got.AvgSellMicros,
			got.RealizedPnLMicros.Int64(), avgSell, realized)
	}
}

func TestHoldingPerformance_aRobinhoodRoundTripIsNetOfBothBridgeFees(t *testing.T) {
	t.Parallel()
	r := newRoundTrip(t)
	r.bought(t, 99_500_000, 500_000)
	r.sold(t)
	r.bridgedBack(t, 110_000_000, 500_000)

	r.want(t, 54_750_000, 9_500_000)
}

func TestHoldingPerformance_aBridgeBackFeeThatLandsBeforeTheSellIsStillNetted(t *testing.T) {
	t.Parallel()
	r := newRoundTrip(t)
	r.bought(t, 99_500_000, 500_000)
	r.bridgedBack(t, 110_000_000, 500_000)
	r.sold(t)

	r.want(t, 54_750_000, 9_500_000)
}

func TestHoldingPerformance_aRepeatedBridgeBackEventDoesNotNetTheFeeTwice(t *testing.T) {
	t.Parallel()
	r := newRoundTrip(t)
	r.bought(t, 99_500_000, 500_000)
	r.sold(t)
	r.bridgedBack(t, 110_000_000, 500_000)
	back := bridgeRun{f: r.f, cabal: r.cabal, swap: r.sell, step: "bridge_back"}
	back.succeeded(t, 110_000_000, 500_000)

	r.want(t, 54_750_000, 9_500_000)
}

func TestHoldingPerformance_aSolanaRoundTripWithNoBridgeFeeIsUnchanged(t *testing.T) {
	t.Parallel()
	r := newRoundTrip(t)
	r.bought(t, 100_000_000, 0)
	r.sold(t)

	r.want(t, 55_000_000, 10_000_000)
}
