package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const voteDecidingCategory = "VOTE_DECIDING"

type VoteDeciding struct {
	VotePorts
	Clock clock.Clock
}

func (VoteDeciding) Name() string { return "vote_deciding" }

func (VoteDeciding) SourceKey(e events.VoteCast) uuid.UUID { return decidingSource(e.ProposalID) }

func (k VoteDeciding) Recipients(ctx context.Context, e events.VoteCast) ([]ids.UserID, error) {
	proposal := ids.ProposalIDFrom(e.ProposalID)
	state, err := k.States.VoteState(ctx, proposal)
	if err != nil {
		return nil, err
	}
	if state.Status != governanceport.StatusOpen || state.Kind == ruleKind || !k.Clock.Now().Before(state.ExpiresAt) ||
		state.Need < 1 || state.Yes != state.Need-1 {
		return nil, nil
	}
	return k.nonVoters(ctx, proposal, state.CabalID)
}

func (k VoteDeciding) Render(ctx context.Context, e events.VoteCast, _ ids.UserID) (Message, error) {
	proposal := ids.ProposalIDFrom(e.ProposalID)
	state, err := k.States.VoteState(ctx, proposal)
	if err != nil {
		return Message{}, err
	}
	facts, err := k.copyOf(ctx, state)
	if err != nil {
		return Message{}, err
	}
	body := decidingBody(
		facts.action,
		facts.asset,
		facts.amount,
		state.Yes,
		state.No,
		closesIn(state.ExpiresAt.Sub(k.Clock.Now())),
	)
	return k.message(
		ctx, voteDecidingKind, voteDecidingCategory, proposal, state, "", body,
	)
}
