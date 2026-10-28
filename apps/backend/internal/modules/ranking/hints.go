package ranking

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
)

func (m *Module) hints() bus.Consumer {
	return bus.Consumer{
		Durable: "ranking_hints",
		Handlers: []bus.HandlerSpec{
			bus.Handle("ranking.hints", adapters.Hints{Hints: m.deps.Bus}.Snapshot),
			bus.Handle("ranking.hints.call_board_written", adapters.Hints{Hints: m.deps.Bus}.CallBoard),
		},
	}
}
