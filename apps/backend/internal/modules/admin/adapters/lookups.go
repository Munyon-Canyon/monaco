package adapters

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ActionLog struct{ DB sqlc.DBTX }

func (a ActionLog) Recent(ctx context.Context, targetType, targetID string, limit int) ([]sqlc.AdminAction, error) {
	rows, err := sqlc.New(a.DB).ListAdminActions(ctx, sqlc.ListAdminActionsParams{
		TargetType: nullableText(&targetType), TargetID: nullableText(&targetID), RowLimit: int64(limit),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, "admin.ActionLog.Recent")
	}
	return rows, nil
}

func (h HTTP) GetAdminUser(
	ctx context.Context, req api.GetAdminUserRequestObject,
) (api.GetAdminUserResponseObject, error) {
	id, err := ids.ParseUserID(req.Id.String())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeUserNotFound, "admin.GetAdminUser")
	}
	view, err := h.Users.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return api.GetAdminUser200JSONResponse(userBody(view)), nil
}

func (h HTTP) FindAdminUser(
	ctx context.Context, req api.FindAdminUserRequestObject,
) (api.FindAdminUserResponseObject, error) {
	view, err := h.Users.ByHandle(ctx, req.Params.Handle)
	if err != nil {
		return nil, err
	}
	return api.FindAdminUser200JSONResponse(userBody(view)), nil
}

func userBody(v app.UserView) api.AdminUser {
	card := v.Card
	history := make([]api.AdminAuthTransition, len(v.History))
	for i, c := range v.History {
		history[i] = authTransition(c)
	}
	cabals := make([]api.AdminCabalRef, len(v.Cabals))
	for i, c := range v.Cabals {
		cabals[i] = api.AdminCabalRef{Id: c.ID.UUID(), Name: c.Name}
	}
	positions := make([]api.AdminSharePosition, len(v.Shares))
	for i, s := range v.Shares {
		positions[i] = api.AdminSharePosition{CabalId: s.CabalID.UUID(), ShareUnits: s.ShareUnits.String()}
	}
	return api.AdminUser{
		Id: card.ID.UUID(), Handle: optionalString(card.Handle), AuthState: api.AdminAuthState(card.AuthState),
		AccountStatus: api.AdminAccountStatus(card.AccountStatus), PhoneVerified: card.PhoneVerified,
		XLinked: card.XLinked, CreatedAt: card.CreatedAt.UTC(), FirstDepositAt: utcOrNil(card.FirstDepositAt),
		Wallet: optionalString(v.Wallet), AuthStateHistory: history, Cabals: cabals, Positions: positions,
		RecentTxns: txnHeaders(v.Txns), RecentAdminActions: actionRecords(v.Actions),
	}
}

func authTransition(c events.UserAuthStateChanged) api.AdminAuthTransition {
	return api.AdminAuthTransition{From: c.From, To: c.To, Cause: c.Cause, At: c.At.UTC()}
}

func txnHeaders(txns []app.TxnHeader) []api.AdminTxnHeader {
	out := make([]api.AdminTxnHeader, len(txns))
	for i, t := range txns {
		out[i] = txnHeader(t)
	}
	return out
}

func txnHeader(t app.TxnHeader) api.AdminTxnHeader {
	var cabal *uuid.UUID
	if t.CabalID != nil {
		id := t.CabalID.UUID()
		cabal = &id
	}
	return api.AdminTxnHeader{
		Id: t.ID, Scope: api.AdminTxnScope(t.Scope), Kind: t.Kind, Status: t.Status, CabalId: cabal,
		TxSignature: optionalString(t.TxSignature), CreatedAt: t.CreatedAt.UTC(),
	}
}

func actionRecords(rows []sqlc.AdminAction) []api.AdminActionRecord {
	out := make([]api.AdminActionRecord, len(rows))
	for i, row := range rows {
		out[i] = record(row)
	}
	return out
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func utcOrNil(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	at := t.UTC()
	return &at
}
