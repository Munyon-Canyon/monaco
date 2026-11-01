package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestHoldsLiveAssets_namesACabalWhoseShareCountCannotPriceAFund(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	q := newQueries(f)
	holds := func(step string, want bool) {
		t.Helper()
		if got, err := q.HoldsLiveAssets(t.Context(), cabal); err != nil || got != want {
			t.Fatalf("%s: HoldsLiveAssets = %v, %v, want %v", step, got, err, want)
		}
	}
	holds("a new cabal", false)

	u, c, err := f.fund(user, cabal, 100_000_000, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	holds("a cabal that holds only cash", false)

	job := f.ids.NewV7()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO cash_out_jobs
		(id, cabal_id, user_id, share_units, payout_micros, slice_micros, status, created_at, updated_at)
		VALUES ($1, $2, $3, 10, 30000000, 30000000, 'completed', $4, $4)`,
		job, cabal.UUID(), user.UUID(), f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	holds("a cash-out that finished", false)
	for _, status := range []string{"started", "selling", "paying"} {
		if _, err := f.pool.Exec(t.Context(),
			`UPDATE cash_out_jobs SET status = $2 WHERE id = $1`, job, status); err != nil {
			t.Fatal(err)
		}
		holds("a "+status+" cash-out", true)
	}
	if _, err := f.pool.Exec(t.Context(), `UPDATE cash_out_jobs SET status = 'failed' WHERE id = $1`, job); err != nil {
		t.Fatal(err)
	}
	holds("a cash-out that failed", false)

	if _, err := f.pool.Exec(t.Context(), `INSERT INTO cabal_positions
		(cabal_id, asset, units, cost_basis_micros, updated_at) VALUES ($1, 'MintHeld', 5, 0, $2)`,
		cabal.UUID(), f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	holds("a cabal that holds a token", true)
}

func TestHoldsLiveAssets_surfacesAReadFailure(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := newQueries(f).HoldsLiveAssets(ctx, f.cabal(t)); err == nil {
		t.Fatal("HoldsLiveAssets with a cancelled context returned no error")
	}
}

type dustCase struct {
	f      fixture
	cabal  ids.CabalID
	asset  market.Asset
	prices *marketfake.PricesFake
}

func newDustCase(t *testing.T) dustCase {
	t.Helper()
	f := newFixture(t)
	prices := &marketfake.PricesFake{}
	prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(30_000_000), f.clock.Now())
	return dustCase{f: f, cabal: f.cabal(t), asset: marketfake.AAPLx(), prices: prices}
}

func (c dustCase) hold(t *testing.T, asset string, units int64) {
	t.Helper()
	if _, err := c.f.pool.Exec(t.Context(), `INSERT INTO cabal_positions
		(cabal_id, asset, units, cost_basis_micros, updated_at) VALUES ($1, $2, $3, 0, $4)
		ON CONFLICT (cabal_id, asset) DO UPDATE SET units = EXCLUDED.units`,
		c.cabal.UUID(), asset, units, c.f.clock.Now()); err != nil {
		t.Fatal(err)
	}
}

func (c dustCase) holds(t *testing.T, step string, q *adapters.Queries, want bool) {
	t.Helper()
	if got, err := q.HoldsLiveAssets(t.Context(), c.cabal); err != nil || got != want {
		t.Fatalf("%s: HoldsLiveAssets = %v, %v, want %v", step, got, err, want)
	}
}

func TestHoldsLiveAssets_ignoresATokenWorthUnderACentAtAFreshPrice(t *testing.T) {
	t.Parallel()
	c := newDustCase(t)
	mint := string(c.asset.Mint.Address())
	fresh := adapterQueries(c.f, c.prices)

	c.hold(t, mint, 10)
	c.holds(t, "a fresh price puts the token under a cent", fresh, false)
	c.hold(t, mint, 100_000)
	c.holds(t, "a fresh price puts the token at three cents", fresh, true)

	c.hold(t, mint, 10)
	stale := &marketfake.PricesFake{}
	stale.Set(c.asset.ID, money.MicrosFromUint64(30_000_000), c.f.clock.Now().Add(-6*time.Minute))
	c.holds(t, "a price older than five minutes", adapterQueries(c.f, stale), true)
}

func TestHoldsLiveAssets_ignoresAnUnpricedTokenUnderTheMinimumSellSize(t *testing.T) {
	t.Parallel()
	c := newDustCase(t)
	mint := string(c.asset.Mint.Address())
	unpriced := newQueries(c.f)

	c.hold(t, mint, 999)
	c.holds(t, "no price and under the minimum sell size", unpriced, false)
	c.hold(t, mint, 1_000)
	c.holds(t, "no price and at the minimum sell size", unpriced, true)
}

func TestHoldsLiveAssets_dustDoesNotHideOtherLiveHoldings(t *testing.T) {
	t.Parallel()
	c := newDustCase(t)
	mint := string(c.asset.Mint.Address())
	q := adapterQueries(c.f, c.prices)
	c.hold(t, mint, 10)
	c.holds(t, "dust alone", q, false)

	user := c.f.user(t)
	job := c.f.ids.NewV7()
	if _, err := c.f.pool.Exec(t.Context(), `INSERT INTO cash_out_jobs
		(id, cabal_id, user_id, share_units, payout_micros, slice_micros, status, created_at, updated_at)
		VALUES ($1, $2, $3, 10, 30000000, 30000000, 'selling', $4, $4)`,
		job, c.cabal.UUID(), user.UUID(), c.f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	c.holds(t, "dust beside a running cash-out", q, true)
	if _, err := c.f.pool.Exec(
		t.Context(),
		`UPDATE cash_out_jobs SET status = 'completed' WHERE id = $1`,
		job,
	); err != nil {
		t.Fatal(err)
	}
	c.holds(t, "dust after the cash-out finished", q, false)

	c.hold(t, "base:0x00000000000000000000000000000000000000aa", 5)
	c.holds(t, "dust beside a token held on Base", q, true)
}

func TestHoldsLiveAssets_aFailedPriceReadIsAnError(t *testing.T) {
	t.Parallel()
	c := newDustCase(t)
	c.hold(t, string(c.asset.Mint.Address()), 10)
	down := adapterQueriesWithReader(c.f, app.PriceReader(
		func(context.Context, []uuid.UUID, time.Time) (map[uuid.UUID]app.Price, error) {
			return nil, errs.New(errs.CodeDBUnavailable, "test.prices")
		}))
	if _, err := down.HoldsLiveAssets(t.Context(), c.cabal); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("HoldsLiveAssets err = %v, want db_unavailable", err)
	}
}
