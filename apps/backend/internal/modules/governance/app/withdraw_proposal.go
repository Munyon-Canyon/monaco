package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type WithdrawProposal struct {
	ProposalID ids.ProposalID
	ActorID    ids.UserID
}

type WithdrawProposalHandler struct {
	uow   *db.UnitOfWork
	clock clock.Clock
}

func NewWithdrawProposalHandler(uow *db.UnitOfWork, c clock.Clock) *WithdrawProposalHandler {
	return &WithdrawProposalHandler{uow: uow, clock: c}
}

func (h *WithdrawProposalHandler) Handle(ctx context.Context, cmd WithdrawProposal) error {
	const op = "governance.WithdrawProposal"
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		p, err := q.LockForWithdraw(ctx, cmd.ProposalID.UUID())
		switch {
		case err != nil:
			return lookupFailed(err, op)
		case p.ProposerID != cmd.ActorID.UUID():
			return errs.New(errs.CodeNotProposer, op)
		case p.Status != string(domain.StatusOpen):
			return errs.New(errs.CodeProposalClosed, op, slog.String("status", p.Status))
		case p.OthersVoted:
			return errs.New(errs.CodeWithdrawNotAllowed, op)
		}
		if _, err := q.Transition(ctx, sqlc.TransitionParams{
			ID: p.ID, FromStatus: string(domain.StatusOpen), ToStatus: string(domain.StatusWithdrawn),
			At: h.clock.Now(),
		}); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		return tx.Events.Append(ctx, events.ProposalWithdrawn{
			V: 1, ProposalID: p.ID, CabalID: p.CabalID, ProposerID: p.ProposerID,
		})
	})
}
