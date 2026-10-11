package market_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestModule_buildsTheBestPriceRouter(t *testing.T) {
	t.Parallel()
	cfg := moduleConfig()
	cfg.Privy.BaseURL, cfg.Timeouts.Privy = "http://privy.invalid", time.Second
	if market.New(module.Deps{Config: cfg, Clock: clock.Real{}}).Router() == nil {
		t.Fatal("Router is nil")
	}
}

func TestModule_quotesEveryVenueThroughOneQuoter(t *testing.T) {
	t.Parallel()
	q := market.New(module.Deps{Config: moduleConfig(), Clock: clock.Real{}}).Quoter()
	_, err := q.Quote(t.Context(), multichain.Listing{Venue: "kyber"}, multichain.SideBuy, money.MicrosFromUint64(1))
	if errs.CodeOf(err) != errs.CodeNoRoute {
		t.Fatalf("err = %v, want no_route for a venue no quoter serves", err)
	}
}
