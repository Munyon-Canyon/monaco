package ranking

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
)

func (m *Module) calls() bus.Consumer {
	c := adapters.Calls{Ballots: m.ports.Ballots, Market: m.ports.Market, DB: m.deps.Pool, Clock: m.deps.Clock}
	return bus.Consumer{
		Durable: "ranking_calls",
		Handlers: []bus.HandlerSpec{
			bus.HandleFetched("ranking.calls.proposal_created", c.FetchCreated, c.ApplyCreated),
			bus.Handle("ranking.calls.vote_cast", c.VoteCast),
			bus.Handle("ranking.calls.vote_retracted", c.VoteRetracted),
			bus.HandleFetched("ranking.calls.proposal_passed", c.FetchPassed, c.ApplyPassed),
			bus.HandleFetched("ranking.calls.proposal_failed", c.FetchFailed, c.ApplyFailed),
			bus.HandleFetched("ranking.calls.proposal_expired", c.FetchExpired, c.ApplyExpired),
			bus.HandleFetched("ranking.calls.trade_confirmed", c.FetchTrade, c.ApplyTrade),
			bus.Handle("ranking.calls.proposal_withdrawn", c.Withdrawn),
			bus.Handle("ranking.calls.proposal_voided", c.Voided),
		},
	}
}
