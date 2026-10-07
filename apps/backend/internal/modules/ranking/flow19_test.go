package ranking_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow19_RunValuation_OK(t *testing.T) {
	t.Parallel()
	flows.F19RunValuationOK(flow19Scenario(t))
	runPinned(t, flows.RankedValuationOKAt)
}

func TestFlow19_RunValuation_PricesStale(t *testing.T) {
	t.Parallel()
	flows.F19RunValuationPricesStale(flow19Scenario(t))
	runPinned(t, flows.RankedValuationPricesStaleAt)
}

func TestFlow19_RunValuation_ConservationBroken(t *testing.T) {
	t.Parallel()
	flows.F19RunValuationConservationBroken(flow19Scenario(t))
}

func TestFlow19_RunValuation_CabalPaused(t *testing.T) {
	t.Parallel()
	flows.F19RunValuationCabalPaused(flow19Scenario(t))
}

func runPinned(t *testing.T, script func(*scenario.Scenario, time.Time)) {
	t.Helper()
	for _, pin := range pinnedTimes(t) {
		t.Run(pin.Name, func(t *testing.T) {
			t.Parallel()
			script(pinnedFlow19(t, pin.At), pin.At)
		})
	}
}

func pinnedTimes(t *testing.T) []flows.RankedPin {
	t.Helper()
	pins, err := flows.RankedOffSessionPins(clock.Real{}.Now())
	if err != nil {
		t.Fatalf("pick the pinned times: %v", err)
	}
	return pins
}

func pinnedFlow19(t *testing.T, at time.Time) *scenario.Scenario {
	t.Helper()
	s := flow19ScenarioOn(t, pinnedClock{at: at, started: clock.Real{}.Now()})
	stock := marketfake.AAPLx()
	_, err := s.DB().Exec(s.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		issuer_tradable, company_key, first_seen_at, updated_at, chain_checked_at)
		VALUES ($1, $2, $3, 8, 'xstocks', 'equity', $4, true, $5, $6, $6, $6)`,
		stock.ID.UUID(), stock.Symbol, stock.Mint.String(), stock.DisplayName, stock.CompanyKey, at)
	if err != nil {
		t.Fatalf("seed AAPLx as an equity: %v", err)
	}
	return s
}

type pinnedClock struct {
	clock.Real
	at, started time.Time
}

func (c pinnedClock) Now() time.Time { return c.at.Add(clock.Real{}.Now().Sub(c.started)) }

type liveMarket struct {
	market.Catalog
	market.Prices
	market.Calendar
}

func flow19Scenario(t *testing.T) *scenario.Scenario {
	t.Helper()
	return flow19ScenarioOn(t, clock.Real{})
}

func flow19ScenarioOn(t *testing.T, clk clock.Clock) *scenario.Scenario {
	t.Helper()
	return scenario.New(t, scenario.WithModules(
		func(d module.Deps) module.Module {
			d.Clock = clk
			return rankingOver(d)
		},
		func(d module.Deps) module.Module {
			d.Config = testkit.Config()
			return treasury.New(d)
		},
	))
}

func rankingOver(d module.Deps) *ranking.Module {
	d.Config = testkit.Config()
	markets := market.New(d)
	return ranking.New(d, ranking.WithPorts(app.Ports{
		Market:   liveMarket{Catalog: markets.Catalog(), Prices: markets.Prices(), Calendar: markets.Calendar()},
		Treasury: treasury.New(d).Queries(),
		Funding:  funding.New(d).Pauses(),
		Cabals:   cabal.New(d).Queries(),
		Users:    identity.New(d).Queries(),
		Previous: sqlc.New(d.Pool), Snapshots: sqlc.New(d.Pool),
	}))
}
