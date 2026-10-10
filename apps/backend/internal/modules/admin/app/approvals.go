package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	ActionCabalBan = "cabal_ban"
	ApprovalTTL    = 24 * time.Hour

	ApprovalPending  = "pending"
	ApprovalApproved = "approved"
	ApprovalRejected = "rejected"
	ApprovalExpired  = "expired"
)

type Approval struct {
	ID            uuid.UUID
	Action        string
	TargetID      uuid.UUID
	RequestedBy   uuid.UUID
	Reason        string
	Status        string
	DecidedBy     *uuid.UUID
	DecidedReason *string
	CreatedAt     time.Time
	DecidedAt     *time.Time
	ExpiresAt     time.Time
}

func NewApproval(row sqlc.AdminApproval) Approval {
	a := Approval{
		ID: row.ID, Action: row.Action, TargetID: row.TargetID, RequestedBy: row.RequestedBy, Reason: row.Reason,
		Status: row.Status, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt,
	}
	if row.DecidedBy.Valid {
		by := uuid.UUID(row.DecidedBy.Bytes)
		a.DecidedBy = &by
	}
	if row.DecidedReason.Valid {
		a.DecidedReason = &row.DecidedReason.String
	}
	if row.DecidedAt.Valid {
		a.DecidedAt = &row.DecidedAt.Time
	}
	return a
}

type CabalStatuses interface {
	Status(ctx context.Context, id ids.CabalID) (cabalport.Status, error)
}

type RequestCabalBan struct {
	CabalID ids.CabalID
	AdminID ids.UserID
	Reason  events.Reason
}

type DecideApproval struct {
	ID      uuid.UUID
	AdminID ids.UserID
	Reason  events.Reason
}

type Approvals struct {
	uow    *db.UnitOfWork
	cabals CabalStatuses
	ids    ids.Generator
	clock  clock.Clock
}

func NewApprovals(uow *db.UnitOfWork, cabals CabalStatuses, g ids.Generator, c clock.Clock) *Approvals {
	return &Approvals{uow: uow, cabals: cabals, ids: g, clock: c}
}

func (a *Approvals) RequestCabalBan(ctx context.Context, cmd RequestCabalBan) (Approval, error) {
	const op = "admin.RequestCabalBan"
	status, err := a.cabals.Status(ctx, cmd.CabalID)
	if err != nil {
		return Approval{}, err
	}
	if status != cabalport.StatusActive {
		return Approval{}, errs.New(errs.CodeCabalNotActive, op)
	}
	now := a.clock.Now()
	row := Approval{
		ID: a.ids.NewV7(), Action: ActionCabalBan, TargetID: cmd.CabalID.UUID(), RequestedBy: cmd.AdminID.UUID(),
		Reason: cmd.Reason.String(), Status: ApprovalPending, CreatedAt: now, ExpiresAt: now.Add(ApprovalTTL),
	}
	err = a.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		if _, err := q.ExpirePendingApproval(ctx, sqlc.ExpirePendingApprovalParams{
			Action: row.Action, TargetID: row.TargetID, Now: now,
		}); err != nil {
			return err
		}
		inserted, err := q.InsertApproval(ctx, sqlc.InsertApprovalParams{
			ID: row.ID, Action: row.Action, TargetID: row.TargetID, RequestedBy: row.RequestedBy, Reason: row.Reason,
			CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt,
		})
		if err != nil {
			return err
		}
		if inserted == 0 {
			return errs.New(errs.CodeApprovalAlreadyPending, op)
		}
		return tx.Events.Append(ctx, events.AdminApprovalRequested{
			V: 1, ApprovalID: row.ID, Action: row.Action, TargetID: row.TargetID, RequestedBy: row.RequestedBy,
			Reason: row.Reason, ExpiresAt: row.ExpiresAt,
		})
	})
	return row, err
}

func (a *Approvals) Approve(ctx context.Context, cmd DecideApproval) (Approval, error) {
	return a.decide(ctx, "admin.ApproveApproval", ApprovalApproved, cmd)
}

func (a *Approvals) Reject(ctx context.Context, cmd DecideApproval) (Approval, error) {
	return a.decide(ctx, "admin.RejectApproval", ApprovalRejected, cmd)
}

func (a *Approvals) decide(ctx context.Context, op, to string, cmd DecideApproval) (Approval, error) {
	now := a.clock.Now()
	var row Approval
	err := a.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		stored, err := q.GetApproval(ctx, cmd.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return errs.New(errs.CodeNotFound, op)
		} else if err != nil {
			return err
		}
		row = NewApproval(stored)
		if to == ApprovalApproved && row.RequestedBy == cmd.AdminID.UUID() {
			return errs.New(errs.CodeSameApprover, op)
		}
		changed, err := q.DecideApproval(ctx, sqlc.DecideApprovalParams{
			ID: row.ID, ToStatus: to, DecidedBy: cmd.AdminID.UUID(), DecidedReason: cmd.Reason.String(),
			DecidedAt: now,
		})
		if err != nil {
			return err
		}
		if changed == 0 {
			return unavailable(op, row, now)
		}
		by, why := cmd.AdminID.UUID(), cmd.Reason.String()
		row.Status, row.DecidedBy, row.DecidedReason, row.DecidedAt = to, &by, &why, &now
		if to != ApprovalApproved {
			return nil
		}
		return tx.Events.Append(ctx, events.AdminCabalBanApproved{
			V: 1, ApprovalID: row.ID, CabalID: row.TargetID, RequestedBy: row.RequestedBy,
			ApprovedBy: cmd.AdminID.UUID(), Reason: cmd.Reason.String(),
		})
	})
	return row, err
}

func unavailable(op string, row Approval, now time.Time) error {
	if row.Status == ApprovalExpired || row.Status == ApprovalPending && !row.ExpiresAt.After(now) {
		return errs.New(errs.CodeApprovalExpired, op)
	}
	return errs.New(errs.CodeApprovalNotPending, op)
}
