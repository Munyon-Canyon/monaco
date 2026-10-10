package app

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type RevokeHandle struct {
	UserID ids.UserID
	Reason events.Reason
}

type ReassignHandle struct {
	Handle   string
	ToUserID ids.UserID
	Reason   events.Reason
}

type HandleAssigner interface {
	AssignHandle(context.Context, sqlc.DBTX, ids.UserID, string, time.Time) error
}

type AdminHandlesDeps struct {
	UoW   *db.UnitOfWork
	Users HandleAssigner
	IDs   ids.Generator
	Clock clock.Clock
	Hints Hints
}

type AdminHandles struct{ d AdminHandlesDeps }

func NewAdminHandles(d AdminHandlesDeps) *AdminHandles { return &AdminHandles{d: d} }

type handleView struct {
	Handle *string `json:"handle"`
}

type handleHolder struct {
	FromUserID *uuid.UUID `json:"from_user_id,omitempty"`
}

type handleTarget struct {
	ToUserID uuid.UUID `json:"to_user_id"`
}

func (h *AdminHandles) Revoke(ctx context.Context, cmd RevokeHandle) error {
	const op = "identity.RevokeHandle"
	admin, err := adminFrom(ctx, op)
	if err != nil {
		return err
	}
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		row, err := q.LockUserHandle(ctx, cmd.UserID.UUID())
		if err != nil {
			return lockFailed(err, op)
		}
		if row.AccountStatus == string(domain.AccountDeleted) {
			return errs.New(errs.CodeUserDeleted, op)
		}
		if !row.Handle.Valid {
			return nil
		}
		at := h.d.Clock.Now()
		if _, err := q.ClearUserHandle(ctx, sqlc.ClearUserHandleParams{ID: row.ID, Now: at}); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		if err := h.cleared(ctx, tx, row.ID); err != nil {
			return err
		}
		audit, err := events.NewAdminAction(
			h.d.IDs.NewV7(), admin, events.AdminActionHandleRevoke, events.AdminTargetUser, row.ID.String(),
			cmd.Reason, handleView{Handle: &row.Handle.String}, handleView{},
		)
		if err != nil {
			return err
		}
		if err := tx.Events.Append(ctx, audit); err != nil {
			return err
		}
		h.hint(tx, row.ID)
		return nil
	})
}

func (h *AdminHandles) Reassign(ctx context.Context, cmd ReassignHandle) error {
	const op = "identity.ReassignHandle"
	admin, err := adminFrom(ctx, op)
	if err != nil {
		return err
	}
	name, err := domain.ParseHandle(cmd.Handle)
	if err != nil {
		return err
	}
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		target, holder, err := h.parties(ctx, q, cmd.ToUserID.UUID(), name.String())
		if err != nil {
			return err
		}
		if holder == target {
			return nil
		}
		if err := h.move(ctx, tx, q, cmd.ToUserID.UUID(), holder, name.String()); err != nil {
			return err
		}
		audit, err := events.NewAdminAction(
			h.d.IDs.NewV7(), admin, events.AdminActionHandleReassign, events.AdminTargetHandle, name.String(),
			cmd.Reason, handleHolder{FromUserID: optionalID(holder)}, handleTarget{ToUserID: cmd.ToUserID.UUID()},
		)
		if err != nil {
			return err
		}
		return tx.Events.Append(ctx, audit)
	})
}

func (h *AdminHandles) parties(
	ctx context.Context, q *sqlc.Queries, target uuid.UUID, handle string,
) (uuid.UUID, uuid.UUID, error) {
	const op = "identity.ReassignHandle"
	rows, err := q.LockHandleParties(ctx, sqlc.LockHandlePartiesParams{Target: target, Handle: handle})
	if err != nil {
		return uuid.Nil, uuid.Nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	var found, holder uuid.UUID
	for _, row := range rows {
		if row.ID == target {
			if row.AccountStatus == string(domain.AccountDeleted) {
				return uuid.Nil, uuid.Nil, errs.New(errs.CodeUserDeleted, op)
			}
			found = row.ID
		}
		if row.Handle.Valid && row.Handle.String == handle {
			holder = row.ID
		}
	}
	if found == uuid.Nil {
		return uuid.Nil, uuid.Nil, errs.New(errs.CodeUserNotFound, op)
	}
	return found, holder, nil
}

func (h *AdminHandles) clear(
	ctx context.Context, tx db.Tx, q *sqlc.Queries, user uuid.UUID, at time.Time,
) error {
	if _, err := q.ClearUserHandle(ctx, sqlc.ClearUserHandleParams{ID: user, Now: at}); err != nil {
		return errs.Wrap(err, errs.CodeInternal, "identity.ReassignHandle")
	}
	if err := h.cleared(ctx, tx, user); err != nil {
		return err
	}
	h.hint(tx, user)
	return nil
}

func (h *AdminHandles) move(
	ctx context.Context, tx db.Tx, q *sqlc.Queries, target, holder uuid.UUID, handle string,
) error {
	at := h.d.Clock.Now()
	if holder != uuid.Nil {
		if err := h.clear(ctx, tx, q, holder, at); err != nil {
			return err
		}
	}
	if err := h.d.Users.AssignHandle(ctx, tx.Queries(), ids.UserIDFrom(target), handle, at); err != nil {
		return err
	}
	err := tx.Events.Append(ctx, events.UserProfileUpdated{
		V: 1, UserID: target, Fields: []string{"handle"}, Handle: handle,
	})
	if err != nil {
		return err
	}
	h.hint(tx, target)
	return nil
}

func (h *AdminHandles) cleared(ctx context.Context, tx db.Tx, user uuid.UUID) error {
	return tx.Events.Append(ctx, events.UserProfileUpdated{V: 1, UserID: user, Fields: []string{"handle"}})
}

func (h *AdminHandles) hint(tx db.Tx, user uuid.UUID) {
	tx.AfterCommit(func(ctx context.Context) {
		h.d.Hints.PublishHint(ctx, "user."+user.String()+".me_changed", nil)
	})
}

func optionalID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
