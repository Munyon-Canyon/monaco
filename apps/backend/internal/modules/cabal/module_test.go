package cabal_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_servesCabalsAndHintsTheCreator(t *testing.T) {
	t.Parallel()
	m := cabal.New(module.Deps{Clock: clock.Real{}, Config: config.Config{
		Privy:    config.Privy{BaseURL: "http://127.0.0.1"},
		Timeouts: config.Timeouts{Privy: time.Second},
	}})
	var routes httpx.Routes
	m.Routes(&routes)
	consumers := m.Consumers()
	if m.Name() != "cabal" || routes.CabalRoutes == nil || len(consumers) != 1 ||
		consumers[0].Durable != "cabal_hints" || m.Pollers() != nil {
		t.Fatalf("module = %s, routes %v, consumers %v, pollers %v",
			m.Name(), routes.CabalRoutes, consumers, m.Pollers())
	}
}
