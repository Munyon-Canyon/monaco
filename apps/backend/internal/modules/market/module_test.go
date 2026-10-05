package market_test

import (
	"net/http"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestModule_buildsARouteChecker(t *testing.T) {
	t.Parallel()
	m := market.New(module.Deps{Config: moduleConfig(), Clock: clock.Real{}})
	if m.RouteChecker() == nil {
		t.Fatal("RouteChecker is nil")
	}
}

func TestModule_isNamedMarketAndMountsTheCatalog(t *testing.T) {
	t.Parallel()
	m := market.New(module.Deps{})
	if m.Name() != "market" {
		t.Fatalf("Name = %q, want market", m.Name())
	}
	if !testkit.Serves(m.Mount, "GET", "/v1/assets") {
		t.Fatal("Routes mounted no catalog")
	}
	if got := m.Consumers(); len(got) != 0 {
		t.Fatalf("Consumers = %v, want none", got)
	}
}

func moduleConfig() config.Config {
	return config.Config{
		XStocks:   config.XStocks{BaseURL: "http://fakes/xstocks"},
		Tessera:   config.Tessera{BaseURL: "http://fakes/tessera"},
		PreStocks: config.PreStocks{BaseURL: "http://fakes/prestocks"},
		CoinGecko: config.CoinGecko{BaseURL: "http://fakes/coingecko"},
		Solana:    config.Solana{RPCURL: "http://fakes/rpc/"},
		Jupiter: config.Jupiter{
			SwapBaseURL: "http://fakes/jupiter/swap/v2", PriceBaseURL: "http://fakes/jupiter/price/v3",
		},
		Market: config.Market{PricePollInterval: 90 * time.Second},
		Timeouts: config.Timeouts{
			XStocks: time.Second, Tessera: time.Second, PreStocks: time.Second, RPC: time.Second,
			JupiterQuote: time.Second, JupiterExecute: time.Minute, CoinGecko: time.Second,
		},
	}
}

func TestModule_pollsTheCatalogHourly(t *testing.T) {
	t.Parallel()
	pollers := market.New(module.Deps{Config: moduleConfig(), HTTPClient: httpclient.New}).Pollers()
	if len(pollers) != 4 || pollers[0].Name() != "market.catalog" || pollers[0].Interval() != time.Hour {
		t.Fatalf("Pollers = %v, want market.catalog every hour first", pollers)
	}
}

func TestModule_samplesPricesAtTheConfiguredInterval(t *testing.T) {
	t.Parallel()
	pollers := market.New(module.Deps{Config: moduleConfig(), HTTPClient: httpclient.New}).Pollers()
	if prices := pollers[1]; prices.Name() != "market.prices" || prices.Interval() != 90*time.Second {
		t.Fatalf("Pollers = %v, want market.prices every 90s", pollers)
	}
}

type unavailable struct{ calls atomic.Int32 }

func (u *unavailable) RoundTrip(r *http.Request) (*http.Response, error) {
	u.calls.Add(1)
	return &http.Response{
		StatusCode: http.StatusServiceUnavailable, Header: http.Header{}, Body: http.NoBody, Request: r,
	}, nil
}

func TestModule_retriesXStocksThreeTimesWithinTheBackoffCeilings(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		rt := &unavailable{}
		var xstocks *httpclient.Client
		deps := module.Deps{
			Config: moduleConfig(),
			HTTPClient: func(name string, opts ...httpclient.Option) *httpclient.Client {
				c := httpclient.New(name, append(opts, httpclient.WithTransport(rt))...)
				if name == "xstocks" {
					xstocks = c
				}
				return c
			},
		}
		market.New(deps).Pollers()
		if xstocks == nil {
			t.Fatal("Pollers built no xstocks client")
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "/assets", nil)
		if err != nil {
			t.Fatal(err)
		}
		start := clock.Real{}.Now()
		resp, err := xstocks.Do(t.Context(), req)
		waited := clock.Real{}.Now().Sub(start)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if err == nil {
			t.Fatal("a 503 on every attempt succeeded")
		}
		if rt.calls.Load() != 3 || waited <= 0 || waited > 750*time.Millisecond {
			t.Fatalf("%d attempts waited %v, want 3 attempts and between 0 and 250ms + 500ms of backoff",
				rt.calls.Load(), waited)
		}
	})
}
