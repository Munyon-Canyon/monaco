package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type LiveSwaps interface {
	HasLiveSwap(ctx context.Context, src trading.Source) (bool, error)
}

type VoidProposal struct {
	ProposalID ids.ProposalID
	Reason     domain.VoidReason
}

type VoidProposalHandler struct {
	uow   *db.UnitOfWork
	reads sqlc.DBTX
	clock clock.Clock
	swaps LiveSwaps
}

func NewVoidProposalHandler(uow *db.UnitOfWork, reads sqlc.DBTX, c clock.Clock, s LiveSwaps) *VoidProposalHandler {
	return &VoidProposalHandler{uow: uow, reads: reads, clock: c, swaps: s}
}

func (h *VoidProposalHandler) Handle(ctx context.Context, cmd VoidProposal) error {
	const op = "governance.VoidProposal"
	actor, ok := auth.ActorFrom(ctx)
	switch {
	case !ok:
		return errs.New(errs.CodeUnauthorized, op)
	case actor.Kind != auth.ActorAdmin && actor.Kind != auth.ActorSystem:
		return errs.New(errs.CodeForbidden, op, slog.String("actor_kind", string(actor.Kind)))
	}
	if _, err := sqlc.New(h.reads).CabalOfProposal(ctx, cmd.ProposalID.UUID()); err != nil {
		return lookupFailed(err, op)
	}
	live, err := h.swaps.HasLiveSwap(ctx, trading.Source{Kind: "proposal", ID: cmd.ProposalID.UUID()})
	if err != nil {
		return err
	}
	if live {
		return errs.New(errs.CodeLiveSwapExists, op)
	}
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		sources := domain.Sources(domain.EventVoid)
		from := make([]string, len(sources))
		for i, s := range sources {
			from[i] = string(s)
		}
		cabal, err := sqlc.New(tx.Queries()).Void(ctx, sqlc.VoidParams{
			ID: cmd.ProposalID.UUID(), FromStatuses: from, ToStatus: string(domain.StatusVoided),
			Reason: string(cmd.Reason), At: h.clock.Now(),
		})
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return errs.New(errs.CodeProposalClosed, op)
		case err != nil:
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		return tx.Events.Append(ctx, events.ProposalVoided{
			V: 1, ProposalID: cmd.ProposalID.UUID(), CabalID: cabal, ActorType: string(actor.Kind),
			Reason: string(cmd.Reason),
		})
	})
}
