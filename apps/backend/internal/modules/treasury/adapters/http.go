package adapters

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

var _ api.StrictServerInterface = HTTP{}

type HTTP struct {
	Reads    *app.ActivityReads
	UserTxns *app.UserTxnReads
	CashOut  *app.CashOutHandler
}

func (h HTTP) GetCashOutPreview(
	ctx context.Context,
	req api.GetCashOutPreviewRequestObject,
) (api.GetCashOutPreviewResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	preview, err := h.CashOut.Preview(ctx, ids.CabalIDFrom(req.Id), user)
	if err != nil {
		return nil, err
	}
	out := api.CashOutPreview{
		SliceMicros: preview.SliceMicros.String(),
		ShareUnits:  preview.ShareUnits.String(),
		MinMicros:   "100000",
	}
	if preview.Pause.Paused {
		out.Pause = &struct {
			Reasons []string  `json:"reasons"`
			Since   time.Time `json:"since"`
		}{Reasons: preview.Pause.Reasons, Since: preview.Pause.Since}
	}
	return api.GetCashOutPreview200JSONResponse(out), nil
}

func (h HTTP) GetCashOutJob(
	ctx context.Context,
	req api.GetCashOutJobRequestObject,
) (api.GetCashOutJobResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	job, err := h.CashOut.Job(ctx, ids.CabalIDFrom(req.Id), user, req.JobId)
	if err != nil {
		return nil, err
	}
	return api.GetCashOutJob200JSONResponse(wireCashOutJob(job)), nil
}

func (h HTTP) PostCashOut(
	ctx context.Context,
	req api.PostCashOutRequestObject,
) (api.PostCashOutResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil || (req.Body.All == nil && req.Body.UsdcMicros == nil) ||
		(req.Body.All != nil && req.Body.UsdcMicros != nil) {
		return nil, errs.New(errs.CodeInvalidInput, "treasury.PostCashOut")
	}
	cmd := app.CashOut{
		CabalID:        ids.CabalIDFrom(req.Id),
		UserID:         user,
		IdempotencyKey: req.Params.IdempotencyKey,
	}
	if req.Body.All != nil {
		cmd.All = *req.Body.All
		if !cmd.All {
			return nil, errs.New(errs.CodeInvalidInput, "treasury.PostCashOut")
		}
	} else {
		cmd.PayoutMicros, err = money.ParseMicros(*req.Body.UsdcMicros)
		if err != nil {
			return nil, err
		}
	}
	job, err := h.CashOut.Handle(ctx, cmd)
	if err != nil {
		return nil, err
	}
	stored := app.CashOutJob{
		ID:           job.ID,
		CabalID:      cmd.CabalID,
		UserID:       user,
		ShareUnits:   job.ShareUnits,
		PayoutMicros: job.PayoutMicros,
		CreatedAt:    job.CreatedAt,
		UpdatedAt:    job.CreatedAt,
		Status:       domain.CashOutStarted,
	}
	return api.PostCashOut202JSONResponse(wireCashOutJob(stored)), nil
}

func wireCashOutJob(job app.CashOutJob) api.CashOutJob {
	return api.CashOutJob{
		Id:             job.ID,
		CabalId:        job.CabalID.UUID(),
		UserId:         job.UserID.UUID(),
		ShareUnits:     job.ShareUnits.String(),
		PayoutMicros:   job.PayoutMicros.String(),
		SellUsdcMicros: job.SellUSDCMicros.String(),
		Status:         api.CashOutJobStatus(job.Status),
		ResultCode:     job.ResultCode,
		CreatedAt:      job.CreatedAt,
		UpdatedAt:      job.UpdatedAt,
	}
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

func (h HTTP) GetMyTxns(
	ctx context.Context,
	req api.GetMyTxnsRequestObject,
) (api.GetMyTxnsResponseObject, error) {
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
		UsdcMicros: strconv.FormatInt(
			v.USDCMicros,
			10,
		), TxSignature: v.TxSignature, CreatedAt: v.CreatedAt,
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
		return ids.UserID{}, errs.New(
			errs.CodeForbidden,
			op,
			slog.String("actor_kind", string(actor.Kind)),
		)
	}
	user, err := ids.ParseUserID(actor.ID)
	if err != nil {
		return ids.UserID{}, errs.Wrap(err, errs.CodeUnauthorized, op)
	}
	return user, nil
}
