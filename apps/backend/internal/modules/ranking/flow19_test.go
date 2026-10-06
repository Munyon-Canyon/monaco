package ranking_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow19_RunValuation_OK(t *testing.T) {
	t.Parallel()
	flows.F19RunValuationOK(flow19Scenario(t))
}

func TestFlow19_RunValuation_PricesStale(t *testing.T) {
	t.Parallel()
	flows.F19RunValuationPricesStale(flow19Scenario(t))
}

func TestFlow19_RunValuation_ConservationBroken(t *testing.T) {
	t.Parallel()
	flows.F19RunValuationConservationBroken(flow19Scenario(t))
}

func TestFlow19_RunValuation_CabalPaused(t *testing.T) {
	t.Parallel()
	flows.F19RunValuationCabalPaused(flow19Scenario(t))
}

type liveMarket struct {
	market.Catalog
	market.Prices
	market.Calendar
}

func flow19Scenario(t *testing.T) *scenario.Scenario {
	t.Helper()
	return scenario.New(t, scenario.WithModules(func(d module.Deps) module.Module {
		return rankingOver(d)
	}))
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
		Previous: sqlc.New(d.Pool),
	}))
}
