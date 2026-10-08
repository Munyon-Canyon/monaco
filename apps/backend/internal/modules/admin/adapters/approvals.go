package adapters

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	defaultApprovalsPage = 50
	defaultApprovalState = app.ApprovalPending
)

func (h HTTP) RequestCabalBan(
	ctx context.Context, req api.RequestCabalBanRequestObject,
) (api.RequestCabalBanResponseObject, error) {
	adminID, reason, err := adminReason(ctx, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	row, err := h.Approvals.RequestCabalBan(ctx, app.RequestCabalBan{
		CabalID: ids.CabalIDFrom(req.Id), AdminID: adminID, Reason: reason,
	})
	if err != nil {
		return nil, err
	}
	return api.RequestCabalBan200JSONResponse(approval(row)), nil
}

func (h HTTP) ApproveAdminApproval(
	ctx context.Context, req api.ApproveAdminApprovalRequestObject,
) (api.ApproveAdminApprovalResponseObject, error) {
	row, err := h.decide(ctx, h.approve, req.Id, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	return api.ApproveAdminApproval200JSONResponse(approval(row)), nil
}

func (h HTTP) RejectAdminApproval(
	ctx context.Context, req api.RejectAdminApprovalRequestObject,
) (api.RejectAdminApprovalResponseObject, error) {
	row, err := h.decide(ctx, h.Approvals.Reject, req.Id, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	return api.RejectAdminApproval200JSONResponse(approval(row)), nil
}

func (h HTTP) approve(ctx context.Context, cmd app.DecideApproval) (app.Approval, error) {
	return h.Approvals.Approve(ctx, app.ApproveCabalBan(cmd))
}

func (h HTTP) decide(
	ctx context.Context, decide func(context.Context, app.DecideApproval) (app.Approval, error),
	id uuid.UUID, text string,
) (app.Approval, error) {
	adminID, reason, err := adminReason(ctx, text)
	if err != nil {
		return app.Approval{}, err
	}
	return decide(ctx, app.DecideApproval{ID: id, AdminID: adminID, Reason: reason})
}

func (h HTTP) GetAdminApprovals(
	ctx context.Context, req api.GetAdminApprovalsRequestObject,
) (api.GetAdminApprovalsResponseObject, error) {
	limit, status := defaultApprovalsPage, defaultApprovalState
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	if req.Params.Status != nil {
		status = string(*req.Params.Status)
	}
	rows, err := sqlc.New(h.Pool).ListApprovals(ctx, sqlc.ListApprovalsParams{
		Status: status, Cursor: nullableUUID(req.Params.Cursor), RowLimit: int64(limit),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, "admin.GetAdminApprovals")
	}
	page := api.AdminApprovals{Items: make([]api.AdminApproval, len(rows))}
	for i, row := range rows {
		page.Items[i] = approval(app.NewApproval(row))
	}
	if len(rows) == limit {
		page.NextCursor = &rows[limit-1].ID
	}
	return api.GetAdminApprovals200JSONResponse(page), nil
}

func approval(row app.Approval) api.AdminApproval {
	return api.AdminApproval{
		Id: row.ID, Action: api.AdminApprovalAction(row.Action), TargetId: row.TargetID,
		RequestedBy: row.RequestedBy, Reason: row.Reason, Status: api.AdminApprovalStatus(row.Status),
		DecidedBy: row.DecidedBy, DecidedReason: row.DecidedReason, CreatedAt: row.CreatedAt.UTC(),
		DecidedAt: utc(row.DecidedAt), ExpiresAt: row.ExpiresAt.UTC(),
	}
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	at := t.UTC()
	return &at
}
