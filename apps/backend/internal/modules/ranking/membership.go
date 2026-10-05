package ranking

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
)

func membership() bus.Consumer {
	return bus.Consumer{
		Durable: "ranking_membership",
		Handlers: []bus.HandlerSpec{
			bus.Handle("ranking.membership", adapters.Membership{}.Joined),
			bus.Handle("ranking.membership.left", adapters.Membership{}.Left),
		},
	}
}
