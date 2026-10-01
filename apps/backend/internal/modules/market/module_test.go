package market_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_isNamedMarketAndMountsNoRoutesOrConsumers(t *testing.T) {
	t.Parallel()
	m := market.New(module.Deps{})
	if m.Name() != "market" {
		t.Fatalf("Name = %q, want market", m.Name())
	}
	var r httpx.Routes
	m.Routes(&r)
	if r != (httpx.Routes{}) {
		t.Fatalf("Routes mounted %+v, want nothing", r)
	}
	if got := m.Consumers(); len(got) != 0 {
		t.Fatalf("Consumers = %v, want none", got)
	}
}

func moduleConfig() config.Config {
	return config.Config{
		XStocks: config.XStocks{BaseURL: "http://fakes/xstocks"},
		Solana:  config.Solana{RPCURL: "http://fakes/rpc/"},
		Jupiter: config.Jupiter{
			SwapBaseURL: "http://fakes/jupiter/swap/v2", PriceBaseURL: "http://fakes/jupiter/price/v3",
		},
		Market: config.Market{PricePollInterval: 90 * time.Second},
		Timeouts: config.Timeouts{
			XStocks: time.Second, RPC: time.Second, JupiterQuote: time.Second, JupiterExecute: time.Minute,
		},
	}
}

func TestModule_pollsTheCatalogHourly(t *testing.T) {
	t.Parallel()
	pollers := market.New(module.Deps{Config: moduleConfig(), HTTPClient: httpclient.New}).Pollers()
	if len(pollers) != 2 || pollers[0].Name() != "market.catalog" || pollers[0].Interval() != time.Hour {
		t.Fatalf("Pollers = %v, want market.catalog every hour first", pollers)
	}
}

func TestModule_samplesPricesAtTheConfiguredInterval(t *testing.T) {
	t.Parallel()
	pollers := market.New(module.Deps{Config: moduleConfig(), HTTPClient: httpclient.New}).Pollers()
	if last := pollers[len(pollers)-1]; last.Name() != "market.prices" || last.Interval() != 90*time.Second {
		t.Fatalf("Pollers = %v, want market.prices every 90s", pollers)
	}
}
