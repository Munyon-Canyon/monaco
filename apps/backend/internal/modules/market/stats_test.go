package market_test

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type holdingModule struct {
	module.Module
	held  map[string]int
	calls *atomic.Int32
	err   error
}

func (holdingModule) Name() string { return "holdingstub" }

func (h holdingModule) CabalsHolding(_ context.Context, asset string) (int, error) {
	h.calls.Add(1)
	return h.held[asset], h.err
}

func newStatsAPI(t *testing.T, held map[string]int) (marketAPI, *atomic.Int32) {
	t.Helper()
	return newStatsAPIFailing(t, held, nil)
}

func newStatsAPIFailing(t *testing.T, held map[string]int, err error) (marketAPI, *atomic.Int32) {
	t.Helper()
	calls := &atomic.Int32{}
	s := newMarketAPIMounting(t, marketWhen(), func(d module.Deps) func(apibase.Mount) {
		m := market.New(d)
		module.NewSet(m, holdingModule{held: held, calls: calls, err: err})
		return m.Mount
	})
	s.seedFixtures(t)
	return s, calls
}

func (s marketAPI) facts(t *testing.T, key string, shares int64) {
	t.Helper()
	_, err := s.pool.Exec(t.Context(), `INSERT INTO company_facts
		(company_key, shares_outstanding, as_of, source, updated_at)
		VALUES ($1, $2, '2026-01-30', 'https://www.sec.gov/Archives/edgar/data/1/000000000026000001/', $3)
		ON CONFLICT (company_key) DO UPDATE SET shares_outstanding = excluded.shares_outstanding`,
		key, shares, s.when)
	if err != nil {
		t.Fatal(err)
	}
}

func (s marketAPI) sessionPrices(t *testing.T, mint string) {
	t.Helper()
	s.price(t, mint, -45*time.Minute, 90_000_000)
	s.price(t, mint, -25*time.Minute, 100_000_000)
	s.price(t, mint, -15*time.Minute, 105_000_000)
	s.price(t, mint, -time.Minute, 110_000_000)
}

func TestStats_aSeededEquityServesTheCapTheOpenAndTheHolderCountWithTheYearCellsNull(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	s, _ := newStatsAPI(t, map[string]int{string(aapl.Mint.Address()): 3})
	s.facts(t, "apple", 15_000_000_000)
	s.sessionPrices(t, aapl.Mint.String())

	rec := s.get(t, "/v1/assets/AAPLx")
	stats := detailOf(t, rec).Stats
	if stats == nil || stats.MarketCapMicros == nil || *stats.MarketCapMicros != "1650000000000000000" {
		t.Fatalf("stats = %+v, want a market cap of 15e9 shares x 110e6 micros", stats)
	}
	if stats.OpenMicros == nil || *stats.OpenMicros != 100_000_000 {
		t.Fatalf("open = %v, want the first session sample 100e6, not the pre-market 90e6", stats.OpenMicros)
	}
	if stats.HeldByCabals != 3 {
		t.Fatalf("held_by_cabals = %d, want 3", stats.HeldByCabals)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"year_low_micros":null`) ||
		!strings.Contains(body, `"year_high_micros":null`) {
		t.Fatalf("body = %s, want both year cells declared and null", body)
	}
}

func TestStats_anEquityWithoutCompanyFactsHasNoCapButKeepsTheOtherCells(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	s, _ := newStatsAPI(t, nil)
	s.sessionPrices(t, aapl.Mint.String())
	if _, err := s.pool.Exec(t.Context(), `DELETE FROM company_facts WHERE company_key = 'apple'`); err != nil {
		t.Fatal(err)
	}

	rec := s.get(t, "/v1/assets/AAPLx")
	stats := detailOf(t, rec).Stats
	if stats == nil || stats.MarketCapMicros != nil || stats.OpenMicros == nil || stats.HeldByCabals != 0 {
		t.Fatalf("stats = %+v, want no cap, an open and 0 holders", stats)
	}
	if !strings.Contains(rec.Body.String(), `"market_cap_micros":null`) {
		t.Fatalf("body = %s, want market_cap_micros null on the wire", rec.Body)
	}
}

func TestStats_anUnpricedEquityHasNoCapAndNoOpen(t *testing.T) {
	t.Parallel()
	s, _ := newStatsAPI(t, nil)
	s.facts(t, "apple", 15_000_000_000)

	stats := detailOf(t, s.get(t, "/v1/assets/AAPLx")).Stats
	if stats == nil || stats.MarketCapMicros != nil || stats.OpenMicros != nil {
		t.Fatalf("stats = %+v, want every price-derived cell null", stats)
	}
}

func TestStats_aPreIPOAssetHasNullStatsAndNeverAsksTreasury(t *testing.T) {
	t.Parallel()
	s, calls := newStatsAPI(t, nil)
	s.preIPO(t, 5, "ACME", mintOne, "Acme Rockets")

	rec := s.get(t, "/v1/assets/ACME")
	if stats := detailOf(t, rec).Stats; stats != nil || !strings.Contains(rec.Body.String(), `"stats":null`) {
		t.Fatalf("stats = %+v body = %s, want stats null", stats, rec.Body)
	}
	if calls.Load() != 0 {
		t.Fatalf("treasury asked %d times for a Pre-IPO asset, want 0", calls.Load())
	}
}

type statsTracer struct{ facts atomic.Int32 }

func (c *statsTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "company_facts") {
		c.facts.Add(1)
	}
	return ctx
}

func (*statsTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestStats_theDetailReadMakesOneMarketQueryAndOneTreasuryCall(t *testing.T) {
	t.Parallel()
	s, _ := newStatsAPI(t, nil)
	aapl := marketfake.AAPLx()
	s.facts(t, "apple", 15_000_000_000)
	s.sessionPrices(t, aapl.Mint.String())
	tracer := &statsTracer{}
	cfg := s.pool.Config()
	cfg.ConnConfig.Tracer = tracer
	db, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var held atomic.Int32
	detail := app.NewDetail(db, app.NewListAssets(db, s.clock)).WithHolding(func(context.Context, string) (int, error) {
		held.Add(1)
		return 1, nil
	})
	view, err := detail.Handle(t.Context(), "AAPLx")
	if err != nil || view.Stats == nil || view.Stats.Open == nil || view.Stats.HeldByCabals != 1 {
		t.Fatalf("view = %+v, %v, want stats with an open and 1 holder", view.Stats, err)
	}
	if tracer.facts.Load() != 1 || held.Load() != 1 {
		t.Fatalf("market stats queries = %d, treasury calls = %d, want 1 and 1", tracer.facts.Load(), held.Load())
	}
}

func TestStats_theOpenOfAClosedMarketIsTheLastSessionsFirstSample(t *testing.T) {
	t.Parallel()
	closed := time.Date(2026, 3, 4, 23, 0, 0, 0, time.UTC)
	s := newMarketAPI(t, closed)
	s.seedFixtures(t)
	mint := marketfake.AAPLx().Mint.String()
	session := time.Date(2026, 3, 4, 14, 30, 0, 0, time.UTC)
	for offset, micros := range map[time.Duration]int64{
		-20 * time.Minute: 90_000_000, 5 * time.Minute: 100_000_000, 2 * time.Hour: 108_000_000,
		6 * time.Hour: 111_000_000, 7 * time.Hour: 150_000_000,
	} {
		_, err := s.pool.Exec(
			t.Context(),
			`INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, $3, 'jupiter')`,
			mint,
			session.Add(offset),
			micros,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	stats := detailOf(t, s.get(t, "/v1/assets/AAPLx")).Stats
	if stats == nil || stats.OpenMicros == nil || *stats.OpenMicros != 100_000_000 {
		t.Fatalf("stats = %+v, want open 100e6 from 14:35 UTC, after the 14:10 pre-market sample", stats)
	}
}

func TestStats_aFailingTreasuryCountFailsTheDetail(t *testing.T) {
	t.Parallel()
	treasury, _ := newStatsAPIFailing(t, nil, errs.New(errs.CodeInternal, "treasury.test"))
	if rec := treasury.get(t, "/v1/assets/AAPLx"); rec.Code != http.StatusInternalServerError {
		t.Fatalf("detail with a failing treasury = %d %s, want 500", rec.Code, rec.Body)
	}
}

func TestStats_aFailingStatsReadFailsTheDetail(t *testing.T) {
	t.Parallel()
	broken, _ := newStatsAPI(t, nil)
	if _, err := broken.pool.Exec(t.Context(), `ALTER TABLE company_facts RENAME TO company_facts_gone`); err != nil {
		t.Fatal(err)
	}
	if rec := broken.get(t, "/v1/assets/AAPLx"); rec.Code != http.StatusInternalServerError {
		t.Fatalf("detail without the facts table = %d %s, want 500", rec.Code, rec.Body)
	}
}

func TestStats_anEarlyCloseSessionOpensThreeAndAHalfHoursBeforeItsClose(t *testing.T) {
	t.Parallel()
	early := time.Date(2026, 11, 27, 16, 0, 0, 0, time.UTC)
	s := newMarketAPI(t, early)
	s.seedFixtures(t)
	mint := marketfake.AAPLx().Mint.String()
	for offset, micros := range map[time.Duration]int64{-90 * time.Minute: 100_000_000, -10 * time.Minute: 101_000_000} {
		_, err := s.pool.Exec(t.Context(),
			`INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, $3, 'jupiter')`,
			mint, early.Add(offset), micros)
		if err != nil {
			t.Fatal(err)
		}
	}
	stats := detailOf(t, s.get(t, "/v1/assets/AAPLx")).Stats
	if stats == nil || stats.OpenMicros == nil || *stats.OpenMicros != 100_000_000 {
		t.Fatalf("stats = %+v, want the open 100e6 sampled at 14:30 UTC on the day after Thanksgiving", stats)
	}
}
