package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/google/uuid"

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
	ids   ids.Generator
	clock clock.Clock
	swaps LiveSwaps
	hints Hints
}

func NewVoidProposalHandler(
	uow *db.UnitOfWork, g ids.Generator, c clock.Clock, s LiveSwaps, hints Hints,
) *VoidProposalHandler {
	return &VoidProposalHandler{uow: uow, ids: g, clock: c, swaps: s, hints: hints}
}

func (h *VoidProposalHandler) Handle(ctx context.Context, cmd VoidProposal) error {
	actor, audit, err := h.prepare(ctx, cmd)
	if err != nil {
		return err
	}
	var cabal uuid.UUID
	err = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		if cabal, err = h.guard(ctx, tx.Queries(), cmd); err != nil {
			return err
		}
		return h.void(ctx, tx, cmd, actor, audit)
	})
	if err != nil {
		return err
	}
	h.hints.ProposalUpdated(ctx, ids.CabalIDFrom(cabal), cmd.ProposalID)
	return nil
}

func (h *VoidProposalHandler) guard(ctx context.Context, q sqlc.DBTX, cmd VoidProposal) (uuid.UUID, error) {
	const op = "governance.VoidProposal"
	cabal, err := sqlc.New(q).LockCabalOfProposal(ctx, cmd.ProposalID.UUID())
	if err != nil {
		return uuid.Nil, lookupFailed(err, op)
	}
	live, err := h.swaps.HasLiveSwap(ctx, trading.Source{Kind: "proposal", ID: cmd.ProposalID.UUID()})
	if err != nil {
		return uuid.Nil, err
	}
	if live {
		return uuid.Nil, errs.New(errs.CodeLiveSwapExists, op)
	}
	return cabal, nil
}

func (h *VoidProposalHandler) Within(ctx context.Context, tx db.Tx, cmd VoidProposal) error {
	actor, audit, err := h.prepare(ctx, cmd)
	if err != nil {
		return err
	}
	cabal, err := h.guard(ctx, tx.Queries(), cmd)
	if err != nil {
		return err
	}
	if err := h.void(ctx, tx, cmd, actor, audit); err != nil {
		return err
	}
	tx.AfterCommit(func(ctx context.Context) {
		h.hints.ProposalUpdated(ctx, ids.CabalIDFrom(cabal), cmd.ProposalID)
	})
	return nil
}

func (h *VoidProposalHandler) prepare(ctx context.Context, cmd VoidProposal) (auth.Actor, voidAudit, error) {
	actor, err := voider(ctx)
	if err != nil {
		return auth.Actor{}, voidAudit{}, err
	}
	audit, err := h.audit(actor, cmd)
	return actor, audit, err
}

func voider(ctx context.Context) (auth.Actor, error) {
	const op = "governance.VoidProposal"
	actor, ok := auth.ActorFrom(ctx)
	switch {
	case !ok:
		return auth.Actor{}, errs.New(errs.CodeUnauthorized, op)
	case actor.Kind != auth.ActorAdmin && actor.Kind != auth.ActorSystem:
		return auth.Actor{}, errs.New(errs.CodeForbidden, op, slog.String("actor_kind", string(actor.Kind)))
	}
	return actor, nil
}

func (h *VoidProposalHandler) void(
	ctx context.Context, tx db.Tx, cmd VoidProposal, actor auth.Actor, audit voidAudit,
) error {
	const op = "governance.VoidProposal"
	sources := domain.Sources(domain.EventVoid)
	from := make([]string, len(sources))
	for i, s := range sources {
		from[i] = string(s)
	}
	voided, err := sqlc.New(tx.Queries()).Void(ctx, sqlc.VoidParams{
		ID: cmd.ProposalID.UUID(), FromStatuses: from, ToStatus: string(domain.StatusVoided),
		Reason: string(cmd.Reason), At: h.clock.Now(),
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return errs.New(errs.CodeProposalClosed, op)
	case err != nil:
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	err = tx.Events.Append(ctx, events.ProposalVoided{
		V: 1, ProposalID: cmd.ProposalID.UUID(), CabalID: voided.CabalID, ActorType: string(actor.Kind),
		Reason: string(cmd.Reason),
	})
	if err != nil || !audit.on {
		return err
	}
	before, _ := json.Marshal(proposalStatus{Status: voided.FromStatus})
	after, _ := json.Marshal(proposalStatus{Status: string(domain.StatusVoided)})
	audit.action.Before, audit.action.After = before, after
	return tx.Events.Append(ctx, audit.action)
}

type proposalStatus struct {
	Status string `json:"status"`
}

type voidAudit struct {
	action events.AdminAction
	on     bool
}

func (h *VoidProposalHandler) audit(actor auth.Actor, cmd VoidProposal) (voidAudit, error) {
	if actor.Kind != auth.ActorAdmin {
		return voidAudit{}, nil
	}
	const op = "governance.VoidProposal.audit"
	admin, err := ids.ParseUserID(actor.ID)
	if err != nil {
		return voidAudit{}, errs.Wrap(err, errs.CodeAdminForbidden, op)
	}
	reason, err := events.NewReason(string(cmd.Reason))
	if err != nil {
		return voidAudit{}, err
	}
	action, err := events.NewAdminAction(
		h.ids.NewV7(), admin, events.AdminActionProposalVoid, events.AdminTargetProposal,
		cmd.ProposalID.String(), reason, nil, nil,
	)
	return voidAudit{action: action, on: true}, err
}
