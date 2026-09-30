package market_test

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type priceRig struct {
	pool   *pgxpool.Pool
	clock  *testkit.Clock
	prices market.Prices
	bucket time.Time
}

func newPriceRig(t *testing.T) *priceRig {
	t.Helper()
	pool := testkit.DB(t)
	bucket := clock.Real{}.Now().UTC().Truncate(2 * time.Minute)
	for _, a := range marketfake.Fixtures() {
		insert(t, pool, stamped(a, bucket), nil)
	}
	clk := testkit.NewClock(bucket.Add(time.Minute))
	return &priceRig{
		pool: pool, clock: clk, bucket: bucket,
		prices: market.New(module.Deps{Pool: pool, Clock: clk}).Prices(),
	}
}

func (r *priceRig) sample(t *testing.T, a market.Asset, micros int64, at time.Time) {
	t.Helper()
	_, err := r.pool.Exec(t.Context(),
		`INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, $3, 'jupiter')`,
		a.Mint.String(), at, micros)
	if err != nil {
		t.Fatal(err)
	}
}

func (r *priceRig) latest(t *testing.T) map[market.AssetID]market.Price {
	t.Helper()
	got, err := r.prices.LatestPrices(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func samePrices(got, want map[market.AssetID]market.Price) bool {
	return maps.EqualFunc(got, want, func(a, b market.Price) bool {
		return a.Micros == b.Micros && a.ObservedAt.Equal(b.ObservedAt)
	})
}

func TestLatestPrices_holdsA25PercentSpikeUntilTheNextSampleConfirmsIt(t *testing.T) {
	t.Parallel()
	r := newPriceRig(t)
	aapl := marketfake.AAPLx()
	r.sample(t, aapl, 200_000_000, r.bucket.Add(-4*time.Minute))
	r.sample(t, aapl, 200_000_000, r.bucket.Add(-2*time.Minute))
	r.sample(t, aapl, 250_000_000, r.bucket)
	held := map[market.AssetID]market.Price{
		aapl.ID: {Micros: usd(200_000_000), ObservedAt: r.bucket.Add(-2 * time.Minute)},
	}
	if got := r.latest(t); !samePrices(got, held) {
		t.Fatalf("LatestPrices after a 25%% spike = %v, want the previous sample %v", got, held)
	}
	confirmed := r.bucket.Add(2 * time.Minute)
	r.sample(t, aapl, 250_000_000, confirmed)
	if got := r.latest(t); !samePrices(got, held) {
		t.Fatalf("LatestPrices before the clock reaches the next sample = %v, want %v", got, held)
	}
	r.clock.Advance(2 * time.Minute)
	want := map[market.AssetID]market.Price{aapl.ID: {Micros: usd(250_000_000), ObservedAt: confirmed}}
	if got := r.latest(t); !samePrices(got, want) {
		t.Fatalf("LatestPrices after the spike is confirmed = %v, want %v", got, want)
	}
}

func TestPricesAsOf_readsEachAskedAssetsAcceptedPriceAtThatTime(t *testing.T) {
	t.Parallel()
	r := newPriceRig(t)
	aapl, tsla, jpst := marketfake.AAPLx(), marketfake.TSLAx(), marketfake.JPSTx()
	lastClose := r.bucket.Add(-time.Hour)
	r.sample(t, aapl, 199_000_000, lastClose.Add(-2*time.Minute))
	r.sample(t, aapl, 200_000_000, lastClose)
	r.sample(t, aapl, 260_000_000, lastClose.Add(2*time.Minute))
	r.sample(t, tsla, 300_000_000, lastClose)
	got, err := r.prices.PricesAsOf(t.Context(), []market.AssetID{aapl.ID, jpst.ID}, lastClose)
	want := map[market.AssetID]market.Price{aapl.ID: {Micros: usd(200_000_000), ObservedAt: lastClose}}
	if err != nil || !samePrices(got, want) {
		t.Fatalf("PricesAsOf(AAPLx, JPSTx at the close) = %v, %v, want only AAPLx at the close %v", got, err, want)
	}
	if got, err := r.prices.PricesAsOf(t.Context(), nil, lastClose); err != nil || len(got) != 0 {
		t.Fatalf("PricesAsOf(no assets) = %v, %v, want nothing", got, err)
	}
	all := r.latest(t)
	if _, priced := all[jpst.ID]; priced || len(all) != 2 {
		t.Fatalf("LatestPrices = %v, want AAPLx and TSLAx only, the unpriced JPSTx absent", all)
	}
}

func TestPrices_aNegativeStoredPriceIsADecodeFailure(t *testing.T) {
	t.Parallel()
	r := newPriceRig(t)
	r.sample(t, marketfake.AAPLx(), -1, r.bucket)
	if _, err := r.prices.LatestPrices(t.Context()); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("LatestPrices over a negative price err = %v, want decode_failed", err)
	}
}

func TestPrices_databaseErrorFailsTheRead(t *testing.T) {
	t.Parallel()
	r := newPriceRig(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.prices.PricesAsOf(ctx, []market.AssetID{marketfake.AAPLx().ID}, r.bucket); err == nil {
		t.Fatal("PricesAsOf on a cancelled context succeeded")
	}
}
