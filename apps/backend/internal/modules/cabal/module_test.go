package cabal_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestModule_servesCabalsAndHintsTheCreator(t *testing.T) {
	t.Parallel()
	m := cabal.New(module.Deps{Clock: clock.Real{}, Config: config.Config{
		Privy:    config.Privy{BaseURL: "http://127.0.0.1", VerificationKey: fakes.PrivyVerificationKey()},
		Timeouts: config.Timeouts{Privy: time.Second},
	}})
	consumers := m.Consumers()
	if m.Name() != "cabal" || !testkit.Serves(m.Mount, "GET", "/v1/cabals") || len(consumers) != 1 ||
		consumers[0].Durable != "cabal_hints" || len(m.Pollers()) != 1 {
		t.Fatalf("module = %s, consumers %v, pollers %v",
			m.Name(), consumers, m.Pollers())
	}
}
