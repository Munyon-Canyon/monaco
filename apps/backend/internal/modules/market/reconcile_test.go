package market_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type reconcileRig struct {
	*backfillRig
	reconcile *app.Reconcile
}

func newReconcileRig(t *testing.T, assets ...market.Asset) *reconcileRig {
	t.Helper()
	return newHotReconcileRig(t, nil, assets...)
}

func newHotReconcileRig(t *testing.T, hot []app.HotMints, assets ...market.Asset) *reconcileRig {
	t.Helper()
	b := newBackfillRig(t)
	seedAssets(t, b.pool, b.clock.Now(), assets...)
	ids := testkit.NewIDs(5)
	return &reconcileRig{
		backfillRig: b,
		reconcile:   app.NewReconcile(db.New(b.pool, ids, b.clock), b.pool, b.clock, b.history, hot...),
	}
}

func (r *reconcileRig) tick(t *testing.T) (poller.Report, error) {
	t.Helper()
	return r.reconcile.Tick(r.ctx(t))
}

func TestReconcile_OneDaysTwoCallPerListedMintInTheSlot(t *testing.T) {
	t.Parallel()
	aapl, tsla := marketfake.AAPLx(), marketfake.TSLAx()
	tsla.IssuerTradable, tsla.PopularRank = false, 0
	r := newReconcileRig(t, aapl, tsla)
	r.history.Put(aapl.Mint, 2,
		sample(r.day.Add(5*time.Hour+4*time.Minute+9*time.Second), 105_000_000),
		sample(r.day.Add(6*time.Hour+33*time.Minute), 106_000_000))

	report, err := r.tick(t)
	if err != nil || report.Scanned != 1 || report.Changed != 2 {
		t.Fatalf("tick = %+v, %v, want 1 mint scanned and 2 rows", report, err)
	}
	calls := r.history.Calls()
	if len(calls) != 1 || calls[0].Mint != aapl.Mint || calls[0].Days != 2 {
		t.Fatalf("calls = %v, want one days=2 call, for the listed mint only", calls)
	}
	got := r.points(t, aapl.Mint)
	if len(got) != 2 || got[r.day.Add(5*time.Hour)] != "105000000/coingecko" ||
		got[r.day.Add(6*time.Hour)] != "106000000/coingecko" {
		t.Fatalf("points = %v, want the two samples truncated to their hours", got)
	}
	if len(r.points(t, tsla.Mint)) != 0 {
		t.Fatal("an unlisted mint gained points")
	}
}

const reconcileSlot = 150

func calledMints(r *reconcileRig) map[market.Mint]bool {
	out := map[market.Mint]bool{}
	for _, c := range r.history.Calls() {
		out[c.Mint] = true
	}
	return out
}

func TestReconcile_aTickCallsEveryHotMintAndAtMostOneSlotOfTheRest(t *testing.T) {
	t.Parallel()
	const listed, unlisted = 1300, 20
	assets := generatedCatalog(t, listed+unlisted)
	for i := listed; i < len(assets); i++ {
		assets[i].IssuerTradable = false
	}
	assets[900].PopularRank = 3
	assets[listed+1].PopularRank = 4
	held := []chain.SolanaAddress{assets[5].Mint.Address(), assets[listed].Mint.Address(), "NotInTheCatalog"}
	proposed := []chain.SolanaAddress{assets[1299].Mint.Address(), assets[5].Mint.Address()}
	hot := []app.HotMints{
		func(context.Context) ([]chain.SolanaAddress, error) { return held, nil },
		func(context.Context) ([]chain.SolanaAddress, error) { return proposed, nil },
	}
	r := newHotReconcileRig(t, hot, assets...)

	report, err := r.tick(t)
	const hotMints = 5
	if err != nil || report.Scanned != hotMints+reconcileSlot || len(r.history.Calls()) != hotMints+reconcileSlot {
		t.Fatalf("tick = %+v, %v with %d calls, want the %d hot mints plus a slot of %d",
			report, err, len(r.history.Calls()), hotMints, reconcileSlot)
	}
	called := calledMints(r)
	for _, i := range []int{5, 900, 1299, listed, listed + 1} {
		if !called[assets[i].Mint] {
			t.Fatalf("hot asset %d was not called", i)
		}
	}
	for i := listed + 2; i < len(assets); i++ {
		if called[assets[i].Mint] {
			t.Fatalf("unlisted, not hot asset %d was called", i)
		}
	}
}

func TestReconcile_theSlotRotatesSoSuccessiveNightsCoverEveryListedMint(t *testing.T) {
	t.Parallel()
	const listed = 1300
	assets := generatedCatalog(t, listed)
	r := newReconcileRig(t, assets...)
	nights := (listed + reconcileSlot - 1) / reconcileSlot
	var previous map[market.Mint]bool
	seen := map[market.Mint]bool{}
	for night := range nights {
		before := len(r.history.Calls())
		if _, err := r.tick(t); err != nil {
			t.Fatal(err)
		}
		tonight := map[market.Mint]bool{}
		for _, c := range r.history.Calls()[before:] {
			tonight[c.Mint] = true
			seen[c.Mint] = true
		}
		if len(tonight) > reconcileSlot {
			t.Fatalf("night %d called %d mints, want at most %d", night, len(tonight), reconcileSlot)
		}
		for m := range previous {
			if tonight[m] {
				t.Fatalf("night %d repeated mint %s from the night before", night, m)
			}
		}
		previous = tonight
		r.clock.Advance(app.ReconcileInterval)
	}
	if len(seen) != listed {
		t.Fatalf("%d nights covered %d mints, want all %d listed", nights, len(seen), listed)
	}
}

func TestReconcile_aFailedHotReadFailsTheTickBeforeAnyCall(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeUpstreamTimeout, "down")
	hot := []app.HotMints{func(context.Context) ([]chain.SolanaAddress, error) { return nil, down }}
	r := newHotReconcileRig(t, hot, marketfake.AAPLx())
	if _, err := r.tick(t); errs.CodeOf(err) != errs.CodeUpstreamTimeout || len(r.history.Calls()) != 0 {
		t.Fatalf("tick = %v with %d calls, want upstream_timeout and no call", err, len(r.history.Calls()))
	}
}

func TestReconcile_fillsHolesAndALiveSampleWins(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	r := newReconcileRig(t, aapl)
	hole, live := r.day.Add(5*time.Hour), r.day.Add(6*time.Hour)
	r.exec(t, `INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, 99000000, 'jupiter')`,
		aapl.Mint.String(), live)
	r.history.Put(aapl.Mint, 2, sample(hole.Add(time.Minute), 105_000_000), sample(live.Add(time.Minute), 106_000_000))

	if report, err := r.tick(t); err != nil || report.Changed != 1 {
		t.Fatalf("tick = %+v, %v, want only the hole filled", report, err)
	}
	if report, err := r.tick(t); err != nil || report.Changed != 0 {
		t.Fatalf("second tick = %+v, %v, want a no-op", report, err)
	}
	got := r.points(t, aapl.Mint)
	if got[hole] != "105000000/coingecko" || got[live] != "99000000/jupiter" || len(got) != 2 {
		t.Fatalf("points = %v, want the hole filled and the live sample kept", got)
	}
}

func TestReconcile_NoKeySkips(t *testing.T) {
	t.Parallel()
	r := newReconcileRig(t, marketfake.AAPLx())
	r.history.WithoutKey()
	report, err := r.tick(t)
	if err != nil || report.Scanned != 0 || len(r.history.Calls()) != 0 {
		t.Fatalf("tick = %+v, %v with %d calls, want nothing done", report, err, len(r.history.Calls()))
	}
	if got := strings.Count(string(r.logs.Bytes()), `"msg":"market.reconcile.skipped_no_key"`); got != 1 {
		t.Fatalf("skipped_no_key logged %d times, want once", got)
	}
}

func TestReconcile_aFailedMintDoesNotStopTheRest(t *testing.T) {
	t.Parallel()
	aapl, tsla := marketfake.AAPLx(), marketfake.TSLAx()
	r := newReconcileRig(t, aapl, tsla)
	r.history.Put(aapl.Mint, 2, sample(r.day, 105_000_000))
	r.history.Put(tsla.Mint, 2, sample(r.day, 305_000_000))
	r.history.FailOnce("MarketChart", errs.New(errs.CodeUpstreamTimeout, "test"))

	report, err := r.tick(t)
	if errs.CodeOf(err) != errs.CodeUpstreamTimeout || report.Scanned != 2 || report.Changed != 1 {
		t.Fatalf("tick = %+v, %v, want upstream_timeout with 2 scanned and the second mint's row", report, err)
	}
}

func TestReconcile_aRateLimitStopsTheTick(t *testing.T) {
	t.Parallel()
	r := newReconcileRig(t, marketfake.AAPLx(), marketfake.TSLAx())
	r.history.Fail("MarketChart", errs.New(errs.CodeCoinGeckoRateLimited, "test"))
	report, err := r.tick(t)
	if errs.CodeOf(err) != errs.CodeCoinGeckoRateLimited || len(r.history.Calls()) != 1 || report.Scanned != 1 {
		t.Fatalf("tick = %+v, %v with %d calls, want one rate limited call and a stop",
			report, err, len(r.history.Calls()))
	}
}

func TestReconcile_databaseFailuresAreReported(t *testing.T) {
	t.Parallel()
	r := newReconcileRig(t, marketfake.AAPLx())
	ctx, cancel := context.WithCancel(r.ctx(t))
	r.history.During(cancel)
	if _, err := r.reconcile.Tick(ctx); err == nil {
		t.Fatal("tick whose write failed succeeded")
	}
	if _, err := r.reconcile.Tick(ctx); err == nil {
		t.Fatal("tick on a cancelled context read the catalog")
	}
}

func TestReconcile_isNamedAndRunsEveryDay(t *testing.T) {
	t.Parallel()
	p := app.NewReconcile(nil, nil, nil, &marketfake.PriceHistoryFake{})
	if p.Name() != "market.reconcile" || p.Interval() != 24*time.Hour {
		t.Fatalf("Reconcile = %s every %v, want market.reconcile every 24h", p.Name(), p.Interval())
	}
}
