package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
)

func (h HTTP) PostReport(
	ctx context.Context, req api.PostReportRequestObject,
) (api.PostReportResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, errs.New(errs.CodeInvalidInput, "social.PostReport")
	}
	cmd := app.CreateReport{
		Reporter: me, Kind: domain.ReportKind(req.Body.Kind), TargetID: req.Body.TargetId,
		Reason: domain.ReportReason(req.Body.Reason),
	}
	if req.Body.Note != nil {
		cmd.Note = *req.Body.Note
	}
	id, err := h.Report.Handle(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return api.PostReport201JSONResponse(api.ReportCreated{Id: id}), nil
}
