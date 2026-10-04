package analytics_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestModule_isNamedAnalyticsAndHasNoRoutesPollersOrConsumerWhileNothingIsExported(t *testing.T) {
	t.Parallel()
	m := analytics.New(module.Deps{})
	if m.Name() != "analytics" || testkit.Serves(m.Mount, "GET", "/v1/assets") || m.Consumers() != nil ||
		m.Pollers() != nil {
		t.Fatalf("module %s serves routes, consumers %v, pollers %v; want none", m.Name(), m.Consumers(),
			m.Pollers())
	}
}
