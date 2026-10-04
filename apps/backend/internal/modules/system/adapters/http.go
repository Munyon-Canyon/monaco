package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/systemapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Record *app.RecordPingHandler
	Reads  sqlc.DBTX
	IDs    ids.Generator
}

var _ api.StrictServerInterface = HTTP{}

func (h HTTP) PostSystemPing(
	ctx context.Context, req api.PostSystemPingRequestObject,
) (api.PostSystemPingResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	note, err := domain.ParseNote(req.Body.Note)
	if err != nil {
		return nil, err
	}
	ping, err := h.Record.Handle(ctx, app.RecordPing{ID: h.IDs.NewV7(), UserID: user, Note: note})
	if err != nil {
		return nil, err
	}
	return api.PostSystemPing201JSONResponse(wire(ping)), nil
}

func (h HTTP) GetSystemPing(
	ctx context.Context, req api.GetSystemPingRequestObject,
) (api.GetSystemPingResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	ping, err := app.GetPing(ctx, h.Reads, req.Id, user)
	if err != nil {
		return nil, err
	}
	return api.GetSystemPing200JSONResponse(wire(ping)), nil
}

func caller(ctx context.Context) (ids.UserID, error) {
	const op = "system.caller"
	actor, ok := auth.ActorFrom(ctx)
	if !ok {
		return ids.UserID{}, errs.New(errs.CodeUnauthorized, op)
	}
	if actor.Kind != auth.ActorUser {
		return ids.UserID{}, errs.New(errs.CodeForbidden, op, slog.String("actor_kind", string(actor.Kind)))
	}
	user, err := ids.ParseUserID(actor.ID)
	if err != nil {
		return ids.UserID{}, errs.Wrap(err, errs.CodeUnauthorized, op)
	}
	return user, nil
}

func wire(p app.Ping) api.Ping {
	return api.Ping{Id: p.ID, Note: p.Note, Echoed: p.Echoed}
}
