package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ResolveReport struct {
	ReportID uuid.UUID
	Reason   events.Reason
	Admin    ids.UserID
}

type ResolveReportHandler struct {
	uow *db.UnitOfWork
	ids ids.Generator
	clk clock.Clock
}

func NewResolveReportHandler(uow *db.UnitOfWork, g ids.Generator, clk clock.Clock) *ResolveReportHandler {
	return &ResolveReportHandler{uow: uow, ids: g, clk: clk}
}

type reportState struct {
	Status string `json:"status"`
}

func (h *ResolveReportHandler) Handle(ctx context.Context, cmd ResolveReport) error {
	const op = "social.ResolveReport"
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		row, err := q.LockReport(ctx, cmd.ReportID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return errs.New(errs.CodeReportNotFound, op, slog.String("report_id", cmd.ReportID.String()))
		case err != nil:
			return errs.Wrap(err, errs.CodeInternal, op)
		case row.Status != "open":
			return nil
		}
		if _, err := q.ResolveReport(ctx, cmd.ReportID); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		audit, err := events.NewAdminAction(
			h.ids.NewV7(), cmd.Admin, events.AdminActionReportResolve, events.AdminTargetReport,
			cmd.ReportID.String(), cmd.Reason, reportState{Status: "open"}, reportState{Status: "resolved"},
		)
		if err != nil {
			return err
		}
		return tx.Events.Append(ctx, audit)
	})
}
