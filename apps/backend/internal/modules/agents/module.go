package agents

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/agents/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/agents/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct{ deps module.Deps }

func New(d module.Deps) *Module { return &Module{deps: d} }

func (*Module) Name() string { return "agents" }

func (*Module) Mount(api.Mount) {}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{}
}

func (*Module) Pollers() []poller.Poller { return nil }

func (m *Module) Queries() port.Queries { return adapters.NewQueries(m.deps.Pool) }

type (
	Queries = port.Queries
	Agent   = port.Agent
)
