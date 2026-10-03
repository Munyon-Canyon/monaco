package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const decideAccessOp = "cabal.DecideAccess"

type Decision string

const (
	Approve Decision = "approve"
	Deny    Decision = "deny"
)

func (d Decision) event() (domain.AccessEvent, error) {
	switch d {
	case Approve:
		return domain.AccessApprove, nil
	case Deny:
		return domain.AccessDeny, nil
	}
	return "", errs.New(errs.CodeInvalidInput, decideAccessOp, slog.String("decision", string(d)))
}

type DecideAccess struct {
	ActorID   ids.UserID
	CabalID   ids.CabalID
	RequestID ids.AccessRequestID
	Decision  Decision
}

type DecideAccessHandler struct {
	uow   *db.UnitOfWork
	clock clock.Clock
}

func NewDecideAccessHandler(uow *db.UnitOfWork, c clock.Clock) *DecideAccessHandler {
	return &DecideAccessHandler{uow: uow, clock: c}
}

func (h *DecideAccessHandler) Handle(ctx context.Context, cmd DecideAccess) (Access, error) {
	event, err := cmd.Decision.event()
	if err != nil {
		return Access{}, err
	}
	var decided Access
	err = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var decideErr error
		decided, decideErr = h.decide(ctx, tx, cmd, event)
		return decideErr
	})
	if err != nil {
		return Access{}, err
	}
	return decided, nil
}

func (h *DecideAccessHandler) decide(
	ctx context.Context, tx db.Tx, cmd DecideAccess, event domain.AccessEvent,
) (Access, error) {
	q := sqlc.New(tx.Queries())
	cabal, actor, err := lockCabal(ctx, q, cmd.CabalID, cmd.ActorID, decideAccessOp)
	if err != nil {
		return Access{}, err
	}
	row, req, err := findRequest(ctx, q, cmd.CabalID, cmd.RequestID, decideAccessOp)
	if err != nil {
		return Access{}, err
	}
	if err := domain.CanDecide(actor, cabal, req); err != nil {
		return Access{}, err
	}
	now := h.clock.Now()
	if event == domain.AccessApprove {
		if err := domain.CanAdmit(cabal, req, now); err != nil {
			return Access{}, err
		}
	}
	decided, err := settleRequest(ctx, tx, settlement{row: row, event: event, actor: cmd.ActorID, at: now},
		decideAccessOp)
	if err != nil || event != domain.AccessApprove {
		return decided, err
	}
	return decided, addMember(ctx, tx, newMember{
		cabalID: cmd.CabalID, userID: req.UserID, rules: cabal.Rules, via: Via(req.Direction),
		requestID: row.ID, at: now,
	}, decideAccessOp)
}
