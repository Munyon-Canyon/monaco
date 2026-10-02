package notify_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_servesDevicesWithNoConsumersOrPollers(t *testing.T) {
	t.Parallel()
	m := notify.New(module.Deps{})
	var routes httpx.Routes
	m.Routes(&routes)
	if m.Name() != "notify" || routes.NotifyRoutes == nil || len(m.Consumers()) != 0 || m.Pollers() != nil {
		t.Fatalf("module = %s, routes %v, consumers %v, pollers %v", m.Name(), routes.NotifyRoutes, m.Consumers(),
			m.Pollers())
	}
}
