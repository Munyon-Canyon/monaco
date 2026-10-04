package notify_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestModule_servesDevicesWithNoConsumersOrPollers(t *testing.T) {
	t.Parallel()
	m := notify.New(module.Deps{})
	if m.Name() != "notify" || !testkit.Serves(m.Mount, "POST", "/v1/devices") || len(m.Consumers()) != 0 ||
		m.Pollers() != nil {
		t.Fatalf("module = %s, consumers %v, pollers %v", m.Name(), m.Consumers(),
			m.Pollers())
	}
}
