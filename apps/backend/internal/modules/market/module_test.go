package market_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
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
	if got := m.Pollers(); len(got) != 0 {
		t.Fatalf("Pollers = %v, want none", got)
	}
}
