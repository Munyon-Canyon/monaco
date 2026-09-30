package analytics_test

import (
	"reflect"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_isNamedAnalyticsAndHasNoRoutesPollersOrConsumerWhileNothingIsExported(t *testing.T) {
	t.Parallel()
	m := analytics.New(module.Deps{})
	var routes httpx.Routes
	m.Routes(&routes)
	if m.Name() != "analytics" || !reflect.DeepEqual(routes, httpx.Routes{}) || m.Consumers() != nil ||
		m.Pollers() != nil {
		t.Fatalf("module %s has routes %+v, consumers %v, pollers %v; want none", m.Name(), routes, m.Consumers(),
			m.Pollers())
	}
}
