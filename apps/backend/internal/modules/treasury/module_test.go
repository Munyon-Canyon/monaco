package treasury_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_consumesConfirmedTradesAndHasNoRoutesOrPollersYet(t *testing.T) {
	t.Parallel()
	m := treasury.New(module.Deps{})
	var routes httpx.Routes
	m.Routes(&routes)
	if m.Name() != "treasury" || routes != (httpx.Routes{}) || m.Pollers() != nil {
		t.Fatalf("module = %s, routes %+v, pollers %v", m.Name(), routes, m.Pollers())
	}
	consumers := m.Consumers()
	if len(consumers) != 1 || consumers[0].Durable != "treasury_trades" || len(consumers[0].Handlers) != 1 ||
		consumers[0].Handlers[0].Name != "treasury.trades" || consumers[0].Handlers[0].Type() != events.TypeTradeConfirmed {
		t.Fatalf("consumers = %+v, want treasury_trades with treasury.trades on trade.confirmed", consumers)
	}
	if _, ok := m.Queries().(adapters.Unwired); !ok {
		t.Fatalf("Queries() = %T, want adapters.Unwired", m.Queries())
	}
}
