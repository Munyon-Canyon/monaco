package adapters

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Reads    *app.ActivityReads
	UserTxns *app.UserTxnReads
}

func (h HTTP) GetCabalActivity(
	ctx context.Context, req api.GetCabalActivityRequestObject,
) (api.GetCabalActivityResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	list := app.ListActivity{CabalID: ids.CabalIDFrom(req.Id), Caller: user}
	if req.Params.Limit != nil {
		list.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		list.Cursor = *req.Params.Cursor
	}
	page, err := h.Reads.List(ctx, list)
	if err != nil {
		return nil, err
	}
	out := api.GetCabalActivity200JSONResponse{Items: make([]api.CabalActivity, len(page.Items))}
	for i, v := range page.Items {
		out.Items[i] = wireActivity(v)
	}
	if page.NextCursor != "" {
		out.NextCursor = &page.NextCursor
	}
	return out, nil
}

func (h HTTP) GetMyTxns(ctx context.Context, req api.GetMyTxnsRequestObject) (api.GetMyTxnsResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	list := app.ListUserTxns{UserID: user}
	if req.Params.Limit != nil {
		list.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		list.Cursor = *req.Params.Cursor
	}
	page, err := h.UserTxns.List(ctx, list)
	if err != nil {
		return nil, err
	}
	out := api.GetMyTxns200JSONResponse{Items: make([]api.UserTxn, len(page.Items))}
	for i, v := range page.Items {
		out.Items[i] = wireUserTxn(v)
	}
	if page.NextCursor != "" {
		out.NextCursor = &page.NextCursor
	}
	return out, nil
}

func wireUserTxn(v app.UserTxnView) api.UserTxn {
	out := api.UserTxn{
		Id: v.ID, Kind: api.UserTxnKind(v.Kind), Status: api.UserTxnStatus(v.Status),
		UsdcMicros: strconv.FormatInt(v.USDCMicros, 10), TxSignature: v.TxSignature, CreatedAt: v.CreatedAt,
	}
	if v.Cabal != nil {
		out.Cabal = &api.UserTxnCabal{Id: v.Cabal.ID.UUID(), Name: v.Cabal.Name}
	}
	return out
}

func wireActivity(v app.ActivityView) api.CabalActivity {
	out := api.CabalActivity{
		Id: v.ID, Kind: api.CabalActivityKind(v.Kind), Status: api.CabalActivityStatus(v.Status),
		UsdcMicros: v.USDCMicros, Units: v.Units, TxSignature: v.TxSignature, OccurredAt: v.OccurredAt,
	}
	if v.Asset != nil {
		out.Asset = &api.ActivityAsset{Symbol: v.Asset.Symbol, Name: v.Asset.Name}
	}
	if v.Actor != nil {
		out.Actor = &api.ActivityActor{
			UserId: v.Actor.UserID.UUID(), Handle: v.Actor.Handle, DisplayName: v.Actor.DisplayName,
		}
	}
	return out
}

func caller(ctx context.Context) (ids.UserID, error) {
	const op = "treasury.caller"
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
