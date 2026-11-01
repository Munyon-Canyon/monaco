package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type BanUser struct {
	UserID ids.UserID
	Reason events.Reason
}

type UnbanUser struct {
	UserID ids.UserID
	Reason events.Reason
}

type AdminAccountStatusDeps struct {
	UoW   *db.UnitOfWork
	IDs   ids.Generator
	Clock clock.Clock
	Hints Hints
}

type AdminAccountStatus struct{ d AdminAccountStatusDeps }

func NewAdminAccountStatus(d AdminAccountStatusDeps) *AdminAccountStatus {
	return &AdminAccountStatus{d: d}
}

type accountStatusView struct {
	AccountStatus string `json:"account_status"`
}

func (h *AdminAccountStatus) Ban(ctx context.Context, cmd BanUser) error {
	return h.move(ctx, "identity.BanUser", cmd.UserID, cmd.Reason, domain.AccountBan, events.AdminActionUserBan)
}

func (h *AdminAccountStatus) Unban(ctx context.Context, cmd UnbanUser) error {
	return h.move(ctx, "identity.UnbanUser", cmd.UserID, cmd.Reason, domain.AccountUnban, events.AdminActionUserUnban)
}

func (h *AdminAccountStatus) move(
	ctx context.Context, op string, user ids.UserID, reason events.Reason, ev domain.AccountEvent,
	kind events.AdminActionKind,
) error {
	admin, err := adminFrom(ctx, op)
	if err != nil {
		return err
	}
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		from, to, err := nextStatus(ctx, tx, q, op, user, ev)
		if err != nil {
			return err
		}
		at := h.d.Clock.Now()
		_, err = q.UpdateAccountStatus(ctx, sqlc.UpdateAccountStatusParams{
			ID: user.UUID(), Next: string(to), Expected: string(from), Now: at,
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		err = tx.Events.Append(ctx, events.UserAccountStatusChanged{
			V: 1, UserID: user.UUID(), From: string(from), To: string(to), Cause: "admin", At: at,
		})
		if err != nil {
			return err
		}
		audit, err := events.NewAdminAction(
			h.d.IDs.NewV7(), admin, kind, events.AdminTargetUser, user.String(), reason,
			accountStatusView{AccountStatus: string(from)}, accountStatusView{AccountStatus: string(to)},
		)
		if err != nil {
			return err
		}
		if err := tx.Events.Append(ctx, audit); err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) {
			h.d.Hints.PublishHint(ctx, "user."+user.String()+".me_changed", nil)
		})
		return nil
	})
}

func nextStatus(
	ctx context.Context, tx db.Tx, q *sqlc.Queries, op string, user ids.UserID, ev domain.AccountEvent,
) (domain.AccountStatus, domain.AccountStatus, error) {
	row, err := q.LockUserStatus(ctx, user.UUID())
	if err != nil {
		return "", "", lockFailed(err, op)
	}
	if ev == domain.AccountBan {
		if err := protectAdminTarget(ctx, tx, op, user); err != nil {
			return "", "", err
		}
	}
	from := domain.AccountStatus(row.AccountStatus)
	to, err := domain.NextAccountStatus(from, ev)
	if err != nil {
		return "", "", refusal(op, from, ev)
	}
	return from, to, nil
}

func adminRank(role string) int {
	switch role {
	case "operator":
		return 3
	case "moderator":
		return 2
	case "viewer":
		return 1
	}
	return 0
}

func protectAdminTarget(ctx context.Context, tx db.Tx, op string, user ids.UserID) error {
	var role string
	err := tx.Queries().QueryRow(ctx,
		`SELECT role FROM admins WHERE user_id = $1 AND revoked_at IS NULL`, user.UUID()).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	actor, _ := auth.ActorFrom(ctx)
	if adminRank(role) >= adminRank(actor.Role) {
		return errs.New(errs.CodeForbidden, op, slog.String("target_admin_role", role))
	}
	return nil
}

func refusal(op string, from domain.AccountStatus, ev domain.AccountEvent) error {
	attrs := slog.String("from", string(from))
	switch {
	case from == domain.AccountDeleted:
		return errs.New(errs.CodeUserDeleted, op, attrs)
	case ev == domain.AccountUnban:
		return errs.New(errs.CodeNotBanned, op, attrs)
	}
	return errs.New(errs.CodeAlreadyBanned, op, attrs)
}

func lockFailed(err error, op string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errs.New(errs.CodeUserNotFound, op)
	}
	return errs.Wrap(err, errs.CodeInternal, op)
}
