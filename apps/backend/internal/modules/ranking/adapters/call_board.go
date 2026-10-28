package adapters

import (
	"context"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type CallBoardTriggers struct{}

func (CallBoardTriggers) CallResolved(ctx context.Context, tx db.Tx, _ events.CallResolved, at time.Time) error {
	return callBoardTrigger(ctx, tx, "call_resolved", at)
}

func (CallBoardTriggers) VoteCast(ctx context.Context, tx db.Tx, _ events.VoteCast, at time.Time) error {
	return callBoardTrigger(ctx, tx, "vote_cast", at)
}

func (CallBoardTriggers) ProposalCreated(
	ctx context.Context, tx db.Tx, _ events.ProposalCreated, at time.Time,
) error {
	return callBoardTrigger(ctx, tx, "proposal_created", at)
}

func (CallBoardTriggers) ProfileUpdated(
	ctx context.Context, tx db.Tx, e events.UserProfileUpdated, at time.Time,
) error {
	if !slices.ContainsFunc(e.Fields, showsOnBoards) {
		return nil
	}
	return callBoardTrigger(ctx, tx, "profile_updated", at)
}

func (CallBoardTriggers) CabalUpdated(ctx context.Context, tx db.Tx, _ events.CabalUpdated, at time.Time) error {
	return callBoardTrigger(ctx, tx, "cabal_updated", at)
}

func callBoardTrigger(ctx context.Context, tx db.Tx, reason string, at time.Time) error {
	return sqlc.New(tx.Queries()).InsertCallBoardTrigger(ctx, sqlc.InsertCallBoardTriggerParams{
		Reason: reason, CreatedAt: at,
	})
}
