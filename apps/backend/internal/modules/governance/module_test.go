package governance_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_isNamedGovernanceAndServesNothingYet(t *testing.T) {
	t.Parallel()
	m := governance.New(module.Deps{})
	var routes httpx.Routes
	m.Routes(&routes)
	if m.Name() != "governance" || len(m.Consumers()) != 0 || m.Pollers() != nil || routes != (httpx.Routes{}) {
		t.Fatalf("module = %s, %d consumers, %v pollers, routes %+v", m.Name(), len(m.Consumers()), m.Pollers(), routes)
	}
}
