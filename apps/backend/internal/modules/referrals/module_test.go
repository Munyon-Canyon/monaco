package referrals_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/referrals"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_registersNoRoutesConsumersOrPollersYet(t *testing.T) {
	t.Parallel()
	m := referrals.New(module.Deps{})
	var routes httpx.Routes
	m.Routes(&routes)
	if m.Name() != "referrals" || routes != (httpx.Routes{}) || len(m.Consumers()) != 0 || m.Pollers() != nil {
		t.Fatalf("module = %s, routes %+v, consumers %v, pollers %v", m.Name(), routes, m.Consumers(), m.Pollers())
	}
}
