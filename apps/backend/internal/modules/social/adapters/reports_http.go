package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
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

func (h HTTP) GetAdminReports(
	ctx context.Context, req api.GetAdminReportsRequestObject,
) (api.GetAdminReportsResponseObject, error) {
	if actor, ok := auth.ActorFrom(ctx); !ok || actor.Kind != auth.ActorAdmin {
		return nil, errs.New(errs.CodeAdminForbidden, "social.GetAdminReports")
	}
	q := app.ReportsQuery{Status: "open", Limit: app.ReportsPageDefault}
	if req.Params.Status != nil {
		q.Status = string(*req.Params.Status)
	}
	if req.Params.Limit != nil {
		q.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		after, err := domain.ParseKeyset(*req.Params.Cursor)
		if err != nil {
			return nil, err
		}
		q.After = &after
	}
	page, err := app.ListReports(ctx, h.Reads, h.Users, q)
	if err != nil {
		return nil, err
	}
	body := api.AdminReports{Items: make([]api.AdminReport, len(page.Items))}
	for i, r := range page.Items {
		body.Items[i] = api.AdminReport{
			Id: r.ID, Reporter: api.AdminReporter{UserId: r.Reporter.UUID(), Handle: r.Handle},
			Kind: api.AdminReportKind(r.Kind), TargetId: r.TargetID, Reason: api.AdminReportReason(r.Reason),
			Note: r.Note, Status: api.AdminReportStatus(r.Status), CreatedAt: r.CreatedAt,
		}
	}
	if page.Next != nil {
		next := page.Next.Encode()
		body.NextCursor = &next
	}
	return api.GetAdminReports200JSONResponse(body), nil
}
