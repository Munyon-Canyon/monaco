package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const inviteMemberOp = "cabal.InviteMember"

type Handles interface {
	UserByHandle(ctx context.Context, handle string) (port.UserCard, error)
}

type InviteMember struct {
	ActorID ids.UserID
	CabalID ids.CabalID
	Handle  string
}

type InviteMemberHandler struct {
	uow     *db.UnitOfWork
	handles Handles
	ids     ids.Generator
	clock   clock.Clock
}

func NewInviteMemberHandler(uow *db.UnitOfWork, handles Handles, g ids.Generator, c clock.Clock) *InviteMemberHandler {
	return &InviteMemberHandler{uow: uow, handles: handles, ids: g, clock: c}
}

func (h *InviteMemberHandler) Handle(ctx context.Context, cmd InviteMember) (Access, error) {
	invitee, err := h.handles.UserByHandle(ctx, cmd.Handle)
	if err != nil {
		return Access{}, err
	}
	var sent Access
	err = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var inviteErr error
		sent, inviteErr = h.invite(ctx, tx, cmd, invitee.ID)
		return inviteErr
	})
	if err != nil {
		return Access{}, err
	}
	return sent, nil
}

func (h *InviteMemberHandler) invite(
	ctx context.Context, tx db.Tx, cmd InviteMember, inviteeID ids.UserID,
) (Access, error) {
	q := sqlc.New(tx.Queries())
	cabal, actors, err := lockCabalFor(ctx, q, cmd.CabalID, inviteMemberOp, cmd.ActorID, inviteeID)
	if err != nil {
		return Access{}, err
	}
	if err := domain.CanInviteUser(actors[0], actors[1], cabal); err != nil {
		return Access{}, err
	}
	id := h.ids.NewV7()
	now := h.clock.Now()
	expiresAt := domain.InviteExpiry(now)
	n, err := q.InsertInvite(ctx, sqlc.InsertInviteParams{
		ID: id, CabalID: cmd.CabalID.UUID(), UserID: inviteeID.UUID(), InvitedBy: cmd.ActorID.UUID(),
		ExpiresAt: expiresAt, Now: now,
	})
	if err != nil {
		return Access{}, errs.Wrap(err, errs.CodeInternal, inviteMemberOp)
	}
	if n == 0 {
		return Access{}, errs.New(errs.CodeRequestPending, inviteMemberOp)
	}
	return Access{ID: id, Direction: string(domain.DirectionInvite), Status: string(domain.AccessPending)},
		tx.Events.Append(ctx, events.CabalAccessRequested{
			V: 1, RequestID: id, CabalID: cmd.CabalID.UUID(), UserID: inviteeID.UUID(),
			Direction: string(domain.DirectionInvite), ActorID: cmd.ActorID.UUID(), ExpiresAt: expiresAt,
		})
}
