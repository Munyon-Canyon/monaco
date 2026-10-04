package system_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/system"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestModule_servesPingsAndRegistersTheEchoConsumer(t *testing.T) {
	t.Parallel()
	m := system.New(module.Deps{})
	consumers := m.Consumers()
	if m.Name() != "system" || !testkit.Serves(m.Mount, "POST", "/v1/system/pings") || m.Pollers() != nil ||
		len(consumers) != 1 {
		t.Fatalf("module = %s, %d consumers, %v pollers", m.Name(), len(consumers),
			m.Pollers())
	}
	c := consumers[0]
	if c.Durable != "system_echo" || len(c.Handlers) != 1 || c.Handlers[0].Name != "system.echo" ||
		c.Handlers[0].Type() != events.TypeSystemPinged {
		t.Fatalf("consumer = %+v, want system_echo running system.echo on system.pinged", c)
	}
}
