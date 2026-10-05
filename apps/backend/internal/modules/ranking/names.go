package ranking

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
)

func (m *Module) names() bus.Consumer {
	return bus.Consumer{
		Durable: "ranking_names",
		Handlers: []bus.HandlerSpec{
			bus.Handle("ranking.names", adapters.Names{Hints: m.deps.Bus}.Handle),
		},
	}
}
