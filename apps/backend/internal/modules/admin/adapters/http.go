package adapters

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
)

const defaultActionsPage = 50

type HTTP struct {
	Pool    *pgxpool.Pool
	Redrive *app.RedriveDeadLetterHandler
	Discard *app.DiscardDeadLetterHandler
}

var _ api.StrictServerInterface = HTTP{}

func (h HTTP) GetAdminMe(ctx context.Context, _ api.GetAdminMeRequestObject) (api.GetAdminMeResponseObject, error) {
	actor, err := admin(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(actor.ID)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeAdminForbidden, "admin.GetAdminMe")
	}
	return api.GetAdminMe200JSONResponse{UserId: id, Role: api.AdminRole(actor.Role)}, nil
}

func (h HTTP) GetAdmins(ctx context.Context, _ api.GetAdminsRequestObject) (api.GetAdminsResponseObject, error) {
	const query = `SELECT user_id, role FROM admins WHERE revoked_at IS NULL ORDER BY granted_at, user_id`
	rows, err := h.Pool.Query(ctx, query)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, "admin.GetAdmins")
	}
	admins, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (api.AdminMe, error) {
		var item api.AdminMe
		if err := row.Scan(&item.UserId, &item.Role); err != nil {
			return api.AdminMe{}, fmt.Errorf("scan admin: %w", err)
		}
		return item, nil
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, "admin.GetAdmins")
	}
	return api.GetAdmins200JSONResponse{Admins: admins}, nil
}

func (h HTTP) GetAdminActions(
	ctx context.Context, req api.GetAdminActionsRequestObject,
) (api.GetAdminActionsResponseObject, error) {
	limit := defaultActionsPage
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	rows, err := sqlc.New(h.Pool).ListAdminActions(ctx, actionFilter(req.Params, limit))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, "admin.GetAdminActions")
	}
	page := api.AdminActions{Items: make([]api.AdminActionRecord, len(rows))}
	for i, row := range rows {
		page.Items[i] = record(row)
	}
	if len(rows) == limit {
		page.NextCursor = &rows[limit-1].ID
	}
	return api.GetAdminActions200JSONResponse(page), nil
}

func actionFilter(p api.GetAdminActionsParams, limit int) sqlc.ListAdminActionsParams {
	return sqlc.ListAdminActionsParams{
		AdminID:    nullableUUID(p.AdminId),
		TargetType: nullableText(p.TargetType),
		TargetID:   nullableText(p.TargetId),
		Action:     nullableText(p.Action),
		Cursor:     nullableUUID(p.Cursor),
		RowLimit:   int64(limit),
	}
}

func record(row sqlc.AdminAction) api.AdminActionRecord {
	var approvedBy *uuid.UUID
	if row.ApprovedBy.Valid {
		id := uuid.UUID(row.ApprovedBy.Bytes)
		approvedBy = &id
	}
	before, after := json.RawMessage(row.Before), json.RawMessage(row.After)
	return api.AdminActionRecord{
		Id: row.ID, AdminId: row.AdminID, Action: api.AdminActionKind(row.Action),
		TargetType: api.AdminTargetType(row.TargetType), TargetId: row.TargetID, Reason: row.Reason,
		Before: &before, After: &after, ApprovedBy: approvedBy, CreatedAt: row.CreatedAt.UTC(),
	}
}

func nullableText[T ~string](v *T) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(*v), Valid: true}
}

func admin(ctx context.Context) (auth.Actor, error) {
	actor, ok := auth.ActorFrom(ctx)
	if !ok || actor.Kind != auth.ActorAdmin {
		return auth.Actor{}, errs.New(errs.CodeAdminForbidden, "admin.actor")
	}
	return actor, nil
}
