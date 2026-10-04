package ranking_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestModule_hasNoRoutesConsumersOrPollersYet(t *testing.T) {
	t.Parallel()
	m := ranking.New(module.Deps{})
	if m.Name() != "ranking" || testkit.Serves(m.Mount, "GET", "/v1/assets") || len(m.Consumers()) != 0 ||
		m.Pollers() != nil {
		t.Fatalf("module = %s, consumers %v, pollers %v", m.Name(), m.Consumers(), m.Pollers())
	}
}
