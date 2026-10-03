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
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type ReportOnrampStatus struct {
	SessionID uuid.UUID
	UserID    ids.UserID
	To        domain.OnrampStatus
	Provider  string
}

type OnrampSession struct {
	ID              uuid.UUID
	Status          domain.OnrampStatus
	SuggestedAmount *money.Micros
	CreatedAt       time.Time
	CompletedAt     *time.Time
}

type ReportOnrampStatusHandler struct {
	uow   *db.UnitOfWork
	clock clock.Clock
	hints HintPublisher
}

func NewReportOnrampStatusHandler(uow *db.UnitOfWork, c clock.Clock, hints HintPublisher) *ReportOnrampStatusHandler {
	return &ReportOnrampStatusHandler{uow: uow, clock: c, hints: hints}
}

func (h *ReportOnrampStatusHandler) Handle(ctx context.Context, cmd ReportOnrampStatus) (OnrampSession, error) {
	const op = "funding.ReportOnrampStatus"
	now := h.clock.Now()
	var out OnrampSession
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		row, err := sqlc.New(tx.Queries()).ReportOnrampStatus(ctx, sqlc.ReportOnrampStatusParams{
			ID: cmd.SessionID, ToStatus: string(cmd.To), Provider: cmd.Provider, Now: now,
			UserID: cmd.UserID.UUID(), Sources: statusTexts(domain.OnrampSources(cmd.To)),
		})
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return errs.New(errs.CodeNotFound, op)
		case err != nil:
			return err
		case row.UserID != cmd.UserID.UUID():
			return errs.New(errs.CodeForbidden, op)
		case !row.Moved:
			return errs.New(errs.CodeOnrampInvalidTransition, op,
				slog.String("from", row.Status), slog.String("to", string(cmd.To)))
		}
		suggested, err := suggestedAmount(row.SuggestedAmountMicros)
		if err != nil {
			return err
		}
		out = OnrampSession{
			ID: cmd.SessionID, Status: cmd.To, SuggestedAmount: suggested, CreatedAt: row.CreatedAt, CompletedAt: &now,
		}
		if err := tx.Events.Append(ctx, statusChanged(cmd.SessionID, row.UserID, domain.OnrampStatus(row.Status),
			cmd.To, suggested, optionalText(cmd.Provider))); err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) {
			h.hints.PublishHint(ctx, "user."+cmd.UserID.String()+".onramp_changed", nil)
		})
		return nil
	})
	if err != nil {
		return OnrampSession{}, err
	}
	return out, nil
}

func statusTexts(statuses []domain.OnrampStatus) []string {
	out := make([]string, len(statuses))
	for i, s := range statuses {
		out[i] = string(s)
	}
	return out
}

func optionalText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func suggestedAmount(raw string) (*money.Micros, error) {
	var m *money.Micros
	if raw == "" {
		return m, nil
	}
	parsed, err := money.ParseMicros(raw)
	if err != nil {
		return m, errs.Wrap(err, errs.CodeInternal, "funding.suggestedAmount")
	}
	return &parsed, nil
}
