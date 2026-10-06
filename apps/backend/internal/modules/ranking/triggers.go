package ranking

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
)

func triggers() bus.Consumer {
	return bus.Consumer{
		Durable: "ranking_triggers",
		Handlers: []bus.HandlerSpec{
			bus.Handle("ranking.triggers", adapters.Triggers{}.Traded),
			bus.Handle("ranking.triggers.funded", adapters.Triggers{}.Funded),
			bus.Handle("ranking.triggers.cashed_out", adapters.Triggers{}.CashedOut),
		},
	}
}
