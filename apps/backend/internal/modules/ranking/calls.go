package ranking

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
)

func (m *Module) calls() bus.Consumer {
	evidence := app.NewCallEvidence(m.ports, m.deps.Pool, m.deps.Clock)
	c := adapters.Calls{}
	return bus.Consumer{
		Durable: "ranking_calls",
		Handlers: []bus.HandlerSpec{
			bus.HandleFetched("ranking.calls.proposal_created", evidence.Opening, c.ApplyCreated),
			bus.Handle("ranking.calls.vote_cast", c.VoteCast),
			bus.HandleFetched("ranking.calls.proposal_passed", evidence.Passed, c.ApplyPassed),
			bus.HandleFetched("ranking.calls.proposal_failed", evidence.Failed, c.ApplyFailed),
			bus.HandleFetched("ranking.calls.proposal_expired", evidence.Expired, c.ApplyExpired),
			bus.HandleFetched("ranking.calls.trade_confirmed", evidence.Fill, c.ApplyFill),
			bus.Handle("ranking.calls.proposal_withdrawn", c.Withdrawn),
			bus.Handle("ranking.calls.proposal_voided", c.Voided),
		},
	}
}
