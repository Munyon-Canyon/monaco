package ranking

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
)

func callBoard() bus.Consumer {
	t := adapters.CallBoardTriggers{}
	return bus.Consumer{
		Durable: "ranking_call_board",
		Handlers: []bus.HandlerSpec{
			bus.Handle("ranking.call_board.call_resolved", t.CallResolved),
			bus.Handle("ranking.call_board.vote_cast", t.VoteCast),
			bus.Handle("ranking.call_board.proposal_created", t.ProposalCreated),
			bus.Handle("ranking.call_board.profile_updated", t.ProfileUpdated),
			bus.Handle("ranking.call_board.cabal_updated", t.CabalUpdated),
		},
	}
}
