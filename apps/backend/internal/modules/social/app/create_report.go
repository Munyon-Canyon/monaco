package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type CreateReport struct {
	Reporter ids.UserID
	Kind     domain.ReportKind
	TargetID uuid.UUID
	Reason   domain.ReportReason
	Note     string
}

type CreateReportDeps struct {
	UoW    *db.UnitOfWork
	Reads  sqlc.DBTX
	Users  Users
	Cabals Cabals
	IDs    ids.Generator
	Clock  clock.Clock
}

type CreateReportHandler struct {
	d CreateReportDeps
}

func NewCreateReportHandler(d CreateReportDeps) *CreateReportHandler {
	return &CreateReportHandler{d: d}
}

func (h *CreateReportHandler) Handle(ctx context.Context, cmd CreateReport) (uuid.UUID, error) {
	const op = "social.CreateReport"
	if !cmd.Kind.Valid() || !cmd.Reason.Valid() || utf8.RuneCountInString(cmd.Note) > domain.MaxReportNote {
		return uuid.Nil, errs.New(errs.CodeInvalidInput, op, slog.String("kind", string(cmd.Kind)),
			slog.String("reason", string(cmd.Reason)))
	}
	if err := h.requireTarget(ctx, cmd.Kind, cmd.TargetID); err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err := h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		queries := sqlc.New(tx.Queries())
		created, err := queries.InsertReport(ctx, sqlc.InsertReportParams{
			ID: h.d.IDs.NewV7(), ReporterID: cmd.Reporter.UUID(), Kind: string(cmd.Kind), TargetID: cmd.TargetID,
			Reason: string(cmd.Reason), Note: cmd.Note, CreatedAt: h.d.Clock.Now().UTC(),
		})
		if errors.Is(err, sql.ErrNoRows) {
			id, err = queries.OpenReportID(ctx, sqlc.OpenReportIDParams{
				ReporterID: cmd.Reporter.UUID(), Kind: string(cmd.Kind), TargetID: cmd.TargetID,
			})
			if err != nil {
				return errs.Wrap(err, errs.CodeInternal, op)
			}
			return nil
		}
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		id = created
		return tx.Events.Append(ctx, events.ReportCreated{
			V: 1, ReportID: id, ReporterID: cmd.Reporter.UUID(), Kind: string(cmd.Kind),
			TargetID: cmd.TargetID, Reason: string(cmd.Reason),
		})
	})
	return id, err
}

func (h *CreateReportHandler) requireTarget(ctx context.Context, kind domain.ReportKind, target uuid.UUID) error {
	const op = "social.CreateReport"
	var err error
	switch kind {
	case domain.ReportMessage:
		_, err = sqlc.New(h.d.Reads).GetChatMessageByID(ctx, target)
	case domain.ReportComment:
		_, err = sqlc.New(h.d.Reads).GetComment(ctx, target)
	case domain.ReportUser:
		return h.requireUser(ctx, ids.UserIDFrom(target))
	case domain.ReportCabal:
		_, err = h.d.Cabals.Cabal(ctx, ids.CabalIDFrom(target))
		if errs.CodeOf(err) == errs.CodeCabalNotFound {
			return errs.Wrap(err, errs.CodeReportTargetNotFound, op, slog.String("kind", string(kind)))
		}
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		return errs.New(errs.CodeReportTargetNotFound, op, slog.String("kind", string(kind)))
	}
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	return nil
}

func (h *CreateReportHandler) requireUser(ctx context.Context, user ids.UserID) error {
	status, err := statusOf(ctx, h.d.Users, user)
	if err != nil {
		return err
	}
	if status == domain.AccountUnknown || status == domain.AccountDeleted {
		return errs.New(errs.CodeReportTargetNotFound, "social.CreateReport", slog.String("kind", "user"))
	}
	return nil
}
