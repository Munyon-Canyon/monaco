package adapters

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
)

type HTTP struct{ Pool *pgxpool.Pool }

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

func admin(ctx context.Context) (auth.Actor, error) {
	actor, ok := auth.ActorFrom(ctx)
	if !ok || actor.Kind != auth.ActorAdmin {
		return auth.Actor{}, errs.New(errs.CodeAdminForbidden, "admin.actor")
	}
	return actor, nil
}
