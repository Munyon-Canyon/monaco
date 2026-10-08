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
			bus.Handle("ranking.triggers.cash_out_started", adapters.Triggers{}.CashOutStarted),
			bus.Handle("ranking.triggers.cash_out_partial", adapters.Triggers{}.CashOutPartial),
			bus.Handle("ranking.triggers.cash_out_failed", adapters.Triggers{}.CashOutFailed),
		},
	}
}
