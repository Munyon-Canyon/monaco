package market_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_buildsTheBestPriceRouter(t *testing.T) {
	t.Parallel()
	cfg := moduleConfig()
	cfg.Privy.BaseURL, cfg.Timeouts.Privy = "http://privy.invalid", time.Second
	if market.New(module.Deps{Config: cfg, Clock: clock.Real{}}).Router() == nil {
		t.Fatal("Router is nil")
	}
}
