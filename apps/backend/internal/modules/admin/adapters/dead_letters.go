package adapters

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	defaultDeadLettersPage = 50
	defaultDeadLetterState = "open"
)

func (h HTTP) GetDeadLetters(
	ctx context.Context, req api.GetDeadLettersRequestObject,
) (api.GetDeadLettersResponseObject, error) {
	limit, status := defaultDeadLettersPage, defaultDeadLetterState
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	if req.Params.Status != nil {
		status = string(*req.Params.Status)
	}
	rows, err := sqlc.New(h.Pool).ListDeadLetters(ctx, sqlc.ListDeadLettersParams{
		Status: status, Consumer: nullableText(req.Params.Consumer), Cursor: nullableUUID(req.Params.Cursor),
		RowLimit: int64(limit),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, "admin.GetDeadLetters")
	}
	page := api.DeadLetters{Items: make([]api.DeadLetter, len(rows))}
	for i, row := range rows {
		page.Items[i] = deadLetter(row)
	}
	if len(rows) == limit {
		page.NextCursor = &rows[limit-1].ID
	}
	return api.GetDeadLetters200JSONResponse(page), nil
}

func (h HTTP) RedriveDeadLetter(
	ctx context.Context, req api.RedriveDeadLetterRequestObject,
) (api.RedriveDeadLetterResponseObject, error) {
	adminID, reason, err := adminReason(ctx, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	if err := h.Redrive.Handle(ctx, app.RedriveDeadLetter{ID: req.Id, AdminID: adminID, Reason: reason}); err != nil {
		return nil, err
	}
	return api.RedriveDeadLetter204Response{}, nil
}

func (h HTTP) DiscardDeadLetter(
	ctx context.Context, req api.DiscardDeadLetterRequestObject,
) (api.DiscardDeadLetterResponseObject, error) {
	adminID, reason, err := adminReason(ctx, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	if err := h.Discard.Handle(ctx, app.DiscardDeadLetter{ID: req.Id, AdminID: adminID, Reason: reason}); err != nil {
		return nil, err
	}
	return api.DiscardDeadLetter204Response{}, nil
}

func adminReason(ctx context.Context, text string) (ids.UserID, events.Reason, error) {
	actor, err := admin(ctx)
	if err != nil {
		return ids.UserID{}, events.Reason{}, err
	}
	adminID, err := ids.ParseUserID(actor.ID)
	if err != nil {
		return ids.UserID{}, events.Reason{}, errs.Wrap(err, errs.CodeAdminForbidden, "admin.actor")
	}
	reason, err := events.NewReason(text)
	return adminID, reason, err
}

func deadLetter(row sqlc.ListDeadLettersRow) api.DeadLetter {
	return api.DeadLetter{
		Id: row.ID, StreamSeq: row.StreamSeq, Consumer: row.Consumer, Handler: row.Handler, Subject: row.Subject,
		EventId: optionalUUID(row.EventID), Code: row.Code, Error: row.Error, Occurrences: row.Occurrences,
		Status: api.DeadLetterStatus(row.Status), FirstSeenAt: row.FirstSeenAt.UTC(),
		LastSeenAt: row.LastSeenAt.UTC(), RedrivenAt: optionalTime(row.RedrivenAt),
		ResolvedAt: optionalTime(row.ResolvedAt), ResolvedBy: optionalUUID(row.ResolvedBy),
		ResolveReason: optionalText(row.ResolveReason),
	}
}

func optionalUUID(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
}

func optionalTime(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	at := v.Time.UTC()
	return &at
}

func optionalText(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}
