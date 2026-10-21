package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type AuthorizeWithdrawal struct {
	ID        uuid.UUID
	UserID    ids.UserID
	SessionID string
	IssuedAt  time.Time
	Request   domain.WithdrawalRequest
}

type WithdrawalAuthorization struct {
	ID        uuid.UUID
	ExpiresAt time.Time
}

type AuthorizeWithdrawalHandler struct {
	uow   *db.UnitOfWork
	clock clock.Clock
}

func NewAuthorizeWithdrawalHandler(uow *db.UnitOfWork, clk clock.Clock) *AuthorizeWithdrawalHandler {
	return &AuthorizeWithdrawalHandler{uow: uow, clock: clk}
}

func (h *AuthorizeWithdrawalHandler) Handle(
	ctx context.Context, cmd AuthorizeWithdrawal,
) (WithdrawalAuthorization, error) {
	const op = "funding.AuthorizeWithdrawal.Handle"
	now := h.clock.Now()
	if cmd.SessionID != "" && now.Sub(cmd.IssuedAt) > domain.StepUpFreshness {
		return WithdrawalAuthorization{}, errs.New(errs.CodeReauthRequired, op,
			slog.Duration("token_age", now.Sub(cmd.IssuedAt).Truncate(time.Second)))
	}
	granted := WithdrawalAuthorization{ID: cmd.ID, ExpiresAt: now.Add(domain.StepUpTTL)}
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return sqlc.New(tx.Queries()).InsertWithdrawalAuthorization(ctx, sqlc.InsertWithdrawalAuthorizationParams{
			ID: cmd.ID, UserID: cmd.UserID.UUID(), SessionID: cmd.SessionID,
			AmountMicros: cmd.Request.Amount.String(), DestinationKind: string(cmd.Request.Kind()),
			Network: string(cmd.Request.Chain()), ToAddress: cmd.Request.Address(),
			CreatedAt: now, ExpiresAt: granted.ExpiresAt,
		})
	})
	if err != nil {
		return WithdrawalAuthorization{}, err
	}
	observability.Info(ctx, observability.FundingWithdrawalAuthorized,
		slog.String("authorization_id", cmd.ID.String()), slog.String("user_id", cmd.UserID.String()),
		slog.String("amount_micros", cmd.Request.Amount.String()), slog.String("network", string(cmd.Request.Chain())))
	return granted, nil
}

func (h *WithdrawHandler) requireStepUp(ctx context.Context, q *sqlc.Queries, cmd Withdraw) error {
	if cmd.Authorization != uuid.Nil {
		return nil
	}
	known, err := q.WithdrewToBefore(ctx, sqlc.WithdrewToBeforeParams{
		UserID: cmd.UserID.UUID(), Network: string(cmd.Request.Chain()), ToAddress: cmd.Request.Address(),
	})
	if err != nil {
		return err
	}
	if cmd.Request.RequiresStepUp(known) {
		return errs.New(errs.CodeStepUpRequired, "funding.Withdraw.requireStepUp",
			slog.Bool("known_destination", known), slog.String("reason", "missing"))
	}
	return nil
}

func (h *WithdrawHandler) consumeAuthorization(ctx context.Context, q *sqlc.Queries, cmd Withdraw) error {
	if cmd.Authorization == uuid.Nil {
		return nil
	}
	now := h.d.Clock.Now()
	moved, err := q.ConsumeWithdrawalAuthorization(ctx, sqlc.ConsumeWithdrawalAuthorizationParams{
		ID: cmd.Authorization, UserID: cmd.UserID.UUID(), SessionID: cmd.SessionID, ConsumedAt: now,
		ConsumedBy: cmd.ID, AmountMicros: cmd.Request.Amount.String(), DestinationKind: string(cmd.Request.Kind()),
		Network: string(cmd.Request.Chain()), ToAddress: cmd.Request.Address(),
	})
	if err != nil || moved == 1 {
		return err
	}
	return h.whyRefused(ctx, q, cmd, now)
}

func (h *WithdrawHandler) whyRefused(ctx context.Context, q *sqlc.Queries, cmd Withdraw, now time.Time) error {
	row, err := q.GetWithdrawalAuthorization(ctx, sqlc.GetWithdrawalAuthorizationParams{
		ID: cmd.Authorization, UserID: cmd.UserID.UUID(),
	})
	return authorizationRefusal(row, err, cmd, now)
}

func authorizationRefusal(row sqlc.GetWithdrawalAuthorizationRow, lookup error, cmd Withdraw, now time.Time) error {
	const op = "funding.Withdraw.consumeAuthorization"
	reason := ""
	switch {
	case errors.Is(lookup, sql.ErrNoRows):
		reason = "unknown"
	case lookup != nil:
		return lookup
	case row.Consumed:
		reason = "used"
	case !row.ExpiresAt.After(now):
		reason = "expired"
	case row.SessionID != cmd.SessionID:
		reason = "other_session"
	}
	if reason != "" {
		return errs.New(errs.CodeStepUpRequired, op, slog.String("reason", reason))
	}
	return errs.New(
		errs.CodeStepUpMismatch,
		op,
		slog.String(
			"authorized_amount_micros",
			row.AmountMicros,
		),
		slog.String("amount_micros", cmd.Request.Amount.String()),
	)
}
