package treasury_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_consumesTradeEventsAndHasNoRoutesOrPollersYet(t *testing.T) {
	t.Parallel()
	m := treasury.New(module.Deps{})
	var routes httpx.Routes
	m.Routes(&routes)
	if m.Name() != "treasury" || routes != (httpx.Routes{}) || m.Pollers() != nil {
		t.Fatalf("module = %s, routes %+v, pollers %v", m.Name(), routes, m.Pollers())
	}
	var got []string
	for _, c := range m.Consumers() {
		for _, h := range c.Handlers {
			got = append(got, c.Durable+" "+h.Name+" "+string(h.Type()))
		}
	}
	want := []string{
		"treasury_trades treasury.trades " + string(events.TypeTradeConfirmed),
		"treasury_activity treasury.activity.submitted " + string(events.TypeTradeSubmitted),
		"treasury_activity treasury.activity.confirmed " + string(events.TypeTradeConfirmed),
		"treasury_activity treasury.activity.failed " + string(events.TypeTradeFailed),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("consumers = %q, want %q", got, want)
	}
	if _, ok := m.Queries().(adapters.Unwired); !ok {
		t.Fatalf("Queries() = %T, want adapters.Unwired", m.Queries())
	}
}
