package treasury_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_hasNoRoutesConsumersOrPollersYet(t *testing.T) {
	t.Parallel()
	m := treasury.New(module.Deps{})
	var routes httpx.Routes
	m.Routes(&routes)
	if m.Name() != "treasury" || routes != (httpx.Routes{}) || m.Consumers() != nil || m.Pollers() != nil {
		t.Fatalf("module = %s, routes %+v, consumers %v, pollers %v", m.Name(), routes, m.Consumers(), m.Pollers())
	}
	if _, ok := m.Queries().(adapters.Unwired); !ok {
		t.Fatalf("Queries() = %T, want adapters.Unwired", m.Queries())
	}
}
