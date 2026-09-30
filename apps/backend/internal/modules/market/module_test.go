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

func TestModule_pollsTheCatalogHourly(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		XStocks:  config.XStocks{BaseURL: "http://fakes/xstocks"},
		Timeouts: config.Timeouts{XStocks: time.Second},
	}
	pollers := market.New(module.Deps{Config: cfg, HTTPClient: httpclient.New}).Pollers()
	if len(pollers) != 1 || pollers[0].Name() != "market.catalog" || pollers[0].Interval() != time.Hour {
		t.Fatalf("Pollers = %v, want market.catalog every hour", pollers)
	}
}
