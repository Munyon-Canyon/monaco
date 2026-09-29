package system_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/system"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_servesPingsAndRegistersTheEchoConsumer(t *testing.T) {
	t.Parallel()
	m := system.New(module.Deps{})
	var routes httpx.Routes
	m.Routes(&routes)
	consumers := m.Consumers()
	if m.Name() != "system" || routes.SystemRoutes == nil || m.Pollers() != nil || len(consumers) != 1 {
		t.Fatalf("module = %s, routes %v, %d consumers, %v pollers", m.Name(), routes.SystemRoutes, len(consumers),
			m.Pollers())
	}
	c := consumers[0]
	if c.Durable != "system_echo" || len(c.Handlers) != 1 || c.Handlers[0].Name != "system.echo" ||
		c.Handlers[0].Type() != events.TypeSystemPinged {
		t.Fatalf("consumer = %+v, want system_echo running system.echo on system.pinged", c)
	}
}
