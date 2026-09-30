package cabal_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_registersNoRoutesConsumersOrPollersYet(t *testing.T) {
	t.Parallel()
	m := cabal.New(module.Deps{})
	var routes httpx.Routes
	m.Routes(&routes)
	if m.Name() != "cabal" || routes != (httpx.Routes{}) || len(m.Consumers()) != 0 || m.Pollers() != nil {
		t.Fatalf("module = %s, routes %+v, consumers %v, pollers %v", m.Name(), routes, m.Consumers(), m.Pollers())
	}
}
