package treasury_test

import (
	"context"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	perfMint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	perfEVM  = "base:0xb200000000000000000000000000000000000c03"
)

type perfNames struct{}

func (perfNames) AssetNames(context.Context) (map[domain.Asset]app.AssetName, error) {
	return map[domain.Asset]app.AssetName{perfMint: {Symbol: "AAPLx", Decimals: 8}}, nil
}

func perfListings(_ context.Context, keys []string, _ time.Time) (map[string]app.ListingQuote, error) {
	out := map[string]app.ListingQuote{}
	for _, key := range keys {
		if key == perfEVM {
			out[key] = app.ListingQuote{
				Symbol: "AAPLon", Network: chain.NetworkBase, Decimals: 6, SharesPerToken: big.NewRat(2, 1),
				Asset: &app.ListedAsset{Mint: perfMint, Symbol: "AAPLx", DisplayName: "Apple"},
			}
		}
	}
	return out, nil
}

func seedPerfTrade(
	t *testing.T, pool *pgxpool.Pool, cabal ids.CabalID, asset, kind, status string, usdc, units int64, at time.Time,
) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `INSERT INTO cabal_activity (id, cabal_id, kind, status, asset,
		usdc_micros, units, occurred_at, updated_at) VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $7)`,
		cabal.UUID(), kind, status, asset, usdc, units, at); err != nil {
		t.Fatal(err)
	}
}

func perfReads(pool *pgxpool.Pool, members member) *app.HoldingPerformanceReads {
	return app.NewHoldingPerformanceReads(pool, members, perfNames{}, perfListings, nil, clock.Real{})
}

func TestHoldingPerformance_replaysBuysAndSellsAcrossEveryChainOfTheStock(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	base := clock.Real{}.Now().UTC().Truncate(time.Second).Add(-24 * time.Hour)
	const share = 100_000_000
	seedPerfTrade(t, pool, c.ID, perfMint, "buy", "confirmed", 200_000_000, 2*share, base)
	seedPerfTrade(t, pool, c.ID, perfMint, "buy", "confirmed", 300_000_000, 2*share, base.Add(time.Hour))
	seedPerfTrade(t, pool, c.ID, perfMint, "sell", "confirmed", 180_000_000, share, base.Add(2*time.Hour))
	seedPerfTrade(t, pool, c.ID, perfMint, "sell", "confirmed", 600_000_000, 3*share, base.Add(4*time.Hour))
	seedPerfTrade(t, pool, c.ID, perfEVM, "buy", "confirmed", 100_000_000, 1_000_000, base.Add(time.Hour))
	seedPerfTrade(t, pool, c.ID, perfEVM, "sell", "confirmed", 71_000_000, 500_000, base.Add(3*time.Hour))
	seedPerfTrade(t, pool, c.ID, perfEVM, "sell", "pending", 999_000_000, 500_000, base.Add(5*time.Hour))
	seedPerfTrade(t, pool, c.ID, perfEVM, "sell", "failed", 999_000_000, 500_000, base.Add(6*time.Hour))
	got, err := perfReads(pool, true).Handle(t.Context(),
		app.HoldingPerformanceQuery{CabalID: c.ID, Caller: c.Creator.ID, Symbol: "AAPLx"})
	if err != nil {
		t.Fatal(err)
	}
	if got.AvgSellMicros == nil || got.AvgSellMicros.Uint64() != 170_200_000 ||
		got.RealizedPnLMicros.Int64() != 301_000_000 {
		t.Fatalf("performance = %v, %d, want avg 170200000 and realized 301000000", got.AvgSellMicros,
			got.RealizedPnLMicros.Int64())
	}
}

func TestHoldingPerformance_readsAPartialSellAndAFullSellOfOneChain(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	base := clock.Real{}.Now().UTC().Truncate(time.Second).Add(-24 * time.Hour)
	reads := perfReads(pool, true)
	read := func() app.HoldingPerformance {
		t.Helper()
		got, err := reads.Handle(t.Context(),
			app.HoldingPerformanceQuery{CabalID: c.ID, Caller: c.Creator.ID, Symbol: "AAPLx"})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := read(); got.AvgSellMicros != nil || got.RealizedPnLMicros.Int64() != 0 {
		t.Fatalf("no trades = %v, %d, want nil and 0", got.AvgSellMicros, got.RealizedPnLMicros.Int64())
	}
	seedPerfTrade(t, pool, c.ID, perfMint, "buy", "confirmed", 300_000_000, 300_000_000, base)
	if got := read(); got.AvgSellMicros != nil || got.RealizedPnLMicros.Int64() != 0 {
		t.Fatalf("bought only = %v, %d, want nil and 0", got.AvgSellMicros, got.RealizedPnLMicros.Int64())
	}
	seedPerfTrade(t, pool, c.ID, perfMint, "sell", "confirmed", 50_000_000, 100_000_000, base.Add(time.Hour))
	if got := read(); got.AvgSellMicros.Uint64() != 50_000_000 || got.RealizedPnLMicros.Int64() != -50_000_000 {
		t.Fatalf("partial sell = %v, %d, want avg 50000000 and realized -50000000", got.AvgSellMicros,
			got.RealizedPnLMicros.Int64())
	}
	seedPerfTrade(t, pool, c.ID, perfMint, "sell", "confirmed", 333_333_334, 200_000_000, base.Add(2*time.Hour))
	if got := read(); got.AvgSellMicros.Uint64() != 127_777_778 || got.RealizedPnLMicros.Int64() != 83_333_334 {
		t.Fatalf("full sell = %v, %d, want avg 127777778 and realized 83333334", got.AvgSellMicros,
			got.RealizedPnLMicros.Int64())
	}
}

func TestHoldingPerformance_refusesANonMemberAndReadsAnUnknownSymbolAsNothingSold(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	seedPerfTrade(t, pool, c.ID, perfMint, "buy", "confirmed", 1, 1, clock.Real{}.Now().UTC())
	query := app.HoldingPerformanceQuery{CabalID: c.ID, Caller: c.Creator.ID, Symbol: "AAPLx"}
	if _, err := perfReads(pool, false).Handle(t.Context(), query); errs.CodeOf(err) != errs.CodeNotCabalMember {
		t.Fatalf("non-member read = %v, want not_cabal_member", err)
	}
	query.Symbol = "NOPE"
	got, err := perfReads(pool, true).Handle(t.Context(), query)
	if err != nil || got.AvgSellMicros != nil || got.RealizedPnLMicros.Int64() != 0 {
		t.Fatalf("unknown symbol = %+v, %v, want nothing sold", got, err)
	}
}

func TestHoldingPerformance_failsOnASellOfMoreThanWasBoughtAndOnUnreadableNames(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	base := clock.Real{}.Now().UTC().Add(-time.Hour)
	seedPerfTrade(t, pool, c.ID, perfMint, "buy", "confirmed", 100, 100, base)
	seedPerfTrade(t, pool, c.ID, perfMint, "sell", "confirmed", 100, 200, base.Add(time.Minute))
	query := app.HoldingPerformanceQuery{CabalID: c.ID, Caller: c.Creator.ID, Symbol: "AAPLx"}
	if _, err := perfReads(pool, true).Handle(t.Context(), query); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("oversold replay = %v, want internal", err)
	}
	down := app.NewHoldingPerformanceReads(pool, member(true), failingNames{}, perfListings, nil, clock.Real{})
	if _, err := down.Handle(t.Context(), query); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("unreadable names = %v, want db_unavailable", err)
	}
	failed := app.NewHoldingPerformanceReads(pool, member(true), perfNames{},
		func(context.Context, []string, time.Time) (map[string]app.ListingQuote, error) {
			return nil, errs.New(errs.CodeDBUnavailable, "test")
		}, nil, clock.Real{})
	seedPerfTrade(t, pool, c.ID, perfEVM, "buy", "confirmed", 1, 1, base)
	if _, err := failed.Handle(t.Context(), query); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("unreadable listings = %v, want db_unavailable", err)
	}
}

func TestHoldingPerformanceRoute_aMemberReadsTheFiguresAndAStrangerIsRefused(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withActivity())
	c := testkit.NewCabal(t, s.DB())
	seedAAPLx(t, s.DB())
	mint := marketfake.AAPLx().Mint.String()
	base := clock.Real{}.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	seedPerfTrade(t, s.DB(), c.ID, mint, "buy", "confirmed", 200_000_000, 200_000_000, base)
	path := "/v1/cabals/" + c.ID.String() + "/holdings/AAPLx/performance"
	s.Given(scenario.AsSeededUser("creator", c.Creator.ID)).
		When(
			scenario.Get(path),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("avg_sell_micros", nil),
			scenario.ExpectJSON("realized_pnl_micros", 0),
		)
	seedPerfTrade(t, s.DB(), c.ID, mint, "sell", "confirmed", 150_000_000, 100_000_000, base.Add(time.Minute))
	s.When(scenario.Get(path)).
		Then(
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("avg_sell_micros", "150000000"),
			scenario.ExpectJSON("realized_pnl_micros", 50_000_000),
		)
	stranger := testkit.SeedUser(t, s.DB(), testkit.UserOpts{})
	s.Given(scenario.AsSeededUser("stranger", stranger.ID)).
		When(scenario.Get(path)).
		Then(scenario.ExpectStatus(http.StatusForbidden), scenario.ExpectProblem(errs.CodeNotCabalMember))
}

type failingMembers struct{}

func (failingMembers) IsMember(context.Context, ids.CabalID, ids.UserID) (bool, error) {
	return false, errs.New(errs.CodeDBUnavailable, "test.IsMember")
}

func TestHoldingPerformance_failsWhenMembershipOrTheTradesCannotBeRead(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	query := app.HoldingPerformanceQuery{CabalID: c.ID, Caller: c.Creator.ID, Symbol: "AAPLx"}
	broken := app.NewHoldingPerformanceReads(pool, failingMembers{}, perfNames{}, perfListings, nil, clock.Real{})
	if _, err := broken.Handle(t.Context(), query); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("membership read failure = %v, want db_unavailable", err)
	}
	unwired := app.NewHoldingPerformanceReads(pool, member(true), perfNames{}, nil, nil, clock.Real{})
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := unwired.Handle(cancelled, query); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("trade read failure = %v, want db_unavailable", err)
	}
	seedPerfTrade(t, pool, c.ID, perfMint, "buy", "confirmed", 100, 0, clock.Real{}.Now().UTC())
	if _, err := unwired.Handle(t.Context(), query); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("trade without units = %v, want internal", err)
	}
}
