package system_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/system"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_registersAsSystemWithNoPollers(t *testing.T) {
	t.Parallel()
	m := system.New(module.Deps{})
	var routes httpx.Routes
	m.Routes(&routes)
	if m.Name() != "system" || len(m.Consumers()) != 0 || m.Pollers() != nil {
		t.Fatalf("module = %s, %d consumers, %v pollers", m.Name(), len(m.Consumers()), m.Pollers())
	}
}
