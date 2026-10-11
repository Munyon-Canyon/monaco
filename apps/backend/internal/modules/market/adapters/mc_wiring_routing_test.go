package adapters

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func routingDeps(key string) module.Deps {
	cfg := config.Config{
		Jupiter:  config.Jupiter{SwapBaseURL: "http://jupiter.invalid", PriceBaseURL: "http://jupiter.invalid"},
		Timeouts: config.Timeouts{JupiterQuote: time.Second, JupiterExecute: time.Second, Privy: time.Second},
		Privy:    config.Privy{BaseURL: "http://privy.invalid", AppID: "app", AppSecret: "secret"},
		Routing: config.Routing{
			UniswapAPIKey: key, UniswapBaseURL: "http://uniswap.invalid", UniswapTimeout: time.Second,
			MinEdgeBps: 10, MaxImpactBps: 300,
		},
	}
	return module.Deps{Config: cfg, Clock: clock.Real{}}
}

func TestMultichainRoutingQuoterRegistersUniswapOnlyWithAKey(t *testing.T) {
	t.Parallel()
	without := RoutingVenues(routingDeps(""))
	if _, ok := without[multichain.VenueUniswap]; ok || without[multichain.VenueJupiter] == nil || len(without) != 1 {
		t.Fatalf("venues without a key = %v", without)
	}
	if MultichainRoutingRouter(routingDeps(""), nil) == nil {
		t.Fatal("no router without an injected client factory")
	}
	deps := routingDeps("uni-key")
	var named []string
	deps.HTTPClient = func(name string, opts ...httpclient.Option) *httpclient.Client {
		named = append(named, name)
		return httpclient.New(name, opts...)
	}
	with := RoutingVenues(deps)
	if with[multichain.VenueUniswap] == nil || len(with) != 2 || len(named) != 1 || named[0] != "uniswap" {
		t.Fatalf("venues with a key = %v, clients %v", with, named)
	}
	if MultichainRoutingRouter(deps, nil) == nil {
		t.Fatal("no router")
	}
}

func TestMultichainRoutingQuoterBuildsItsVenuesOnFirstUse(t *testing.T) {
	t.Parallel()
	q := MultichainRoutingQuoter(routingDeps(""))
	l := multichain.Listing{Venue: "kyber"}
	if _, err := q.Quote(
		t.Context(),
		l,
		multichain.SideBuy,
		money.MicrosFromUint64(1),
	); errs.CodeOf(
		err,
	) != errs.CodeNoRoute {
		t.Fatalf("err = %v, want no_route from the built venues", err)
	}
}
