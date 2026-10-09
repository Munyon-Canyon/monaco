package market_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func priceScenario(t *testing.T, interval, quoteTimeout time.Duration) *scenario.Scenario {
	t.Helper()
	upstreams := fakes.New()
	srv := httptest.NewServer(upstreams)
	t.Cleanup(srv.Close)
	return scenario.New(t,
		scenario.WithPrivy(upstreams, "app"),
		scenario.WithModules(func(d module.Deps) module.Module {
			cfg := moduleConfig()
			cfg.Jupiter.PriceBaseURL = srv.URL + "/jupiter/price/v3"
			cfg.Jupiter.SwapBaseURL = srv.URL + "/jupiter/swap/v2"
			cfg.XStocks.BaseURL = srv.URL + "/xstocks"
			cfg.Tessera.BaseURL = srv.URL + "/tessera"
			cfg.PreStocks.BaseURL = srv.URL + "/prestocks"
			cfg.Jupiter.APIKey = "test-key"
			cfg.Market.PricePollInterval = interval
			cfg.Timeouts.JupiterQuote = quoteTimeout
			d.Config = cfg
			d.HTTPClient = httpclient.New
			return market.New(d)
		}),
	)
}

func TestFlow18_SamplePrices_OK(t *testing.T) {
	t.Parallel()
	flows.F18SamplePricesOK(priceScenario(t, time.Minute, 30*time.Second))
}

func TestFlow18_SamplePrices_JupiterUnavailable(t *testing.T) {
	t.Parallel()
	flows.F18SamplePricesJupiterUnavailable(priceScenario(t, time.Second, 200*time.Millisecond))
}

func TestFlow18_SamplePrices_UpstreamTimeout(t *testing.T) {
	t.Parallel()
	flows.F18SamplePricesUpstreamTimeout(priceScenario(t, time.Second, 200*time.Millisecond))
}
