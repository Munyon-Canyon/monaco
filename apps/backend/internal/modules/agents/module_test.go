package agents_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/agents"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestModule_hasNoRoutesConsumersOrPollersYet(t *testing.T) {
	t.Parallel()
	m := agents.New(module.Deps{})
	if m.Name() != "agents" || len(m.Consumers()) != 0 || m.Pollers() != nil ||
		testkit.Serves(m.Mount, "GET", "/v1/agent") {
		t.Fatalf("module = %s, consumers %v, pollers %v", m.Name(), m.Consumers(), m.Pollers())
	}
}
