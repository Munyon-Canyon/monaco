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

func (h HTTP) GetAdminCabal(
	ctx context.Context, req api.GetAdminCabalRequestObject,
) (api.GetAdminCabalResponseObject, error) {
	id, err := ids.ParseCabalID(req.Id.String())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeCabalNotFound, "admin.GetAdminCabal")
	}
	view, err := h.Cabals.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return api.GetAdminCabal200JSONResponse(cabalBody(view)), nil
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

func cabalBody(v app.CabalView) api.AdminCabal {
	members := make([]api.AdminCabalMember, len(v.Members))
	for i, m := range v.Members {
		members[i] = api.AdminCabalMember{
			UserId: m.View.UserID.UUID(), Handle: optionalString(m.Handle), Role: string(m.View.Role),
			CanVote: m.View.CanVote, JoinedAt: m.View.JoinedAt.UTC(), ShareUnits: m.ShareUnits.String(),
		}
	}
	holdings := make([]api.AdminHolding, len(v.Holdings))
	for i, h := range v.Holdings {
		holdings[i] = api.AdminHolding{Mint: h.Mint, Symbol: optionalString(h.Symbol), Units: h.Units.String()}
	}
	pot, nav, valuedAt := valuation(v.Value)
	pauses := make([]api.AdminCabalPause, len(v.Pauses))
	for i, p := range v.Pauses {
		pauses[i] = api.AdminCabalPause{
			Reason: api.AdminCabalPauseReason(p.Reason),
			Scope:  api.AdminCabalPauseScope(p.Scope),
			Since:  p.Since.UTC(),
		}
	}
	return api.AdminCabal{
		PotMicros:          pot,
		NavPerShareMicros:  nav,
		ValuedAt:           valuedAt,
		Id:                 v.Cabal.ID.UUID(),
		Name:               v.Cabal.Name,
		Status:             api.AdminCabalStatus(v.Cabal.Status),
		CreatedAt:          v.Cabal.CreatedAt.UTC(),
		CreatorId:          v.Cabal.CreatorID.UUID(),
		Rules:              rulesBody(v),
		MemberCount:        int64(v.Cabal.MemberCount),
		Members:            members,
		TreasuryAddress:    string(v.Treasury.Address),
		Positions:          holdings,
		Pauses:             pauses,
		RecentTxns:         txnHeaders(v.Txns),
		RecentAdminActions: actionRecords(v.Actions),
	}
}

func valuation(v *app.Valuation) (pot, nav *string, valuedAt *time.Time) {
	if v == nil {
		return nil, nil, nil
	}
	p, n, at := v.Pot.String(), v.NavPerShare.String(), v.At.UTC()
	return &p, &n, &at
}

func rulesBody(v app.CabalView) api.AdminCabalRules {
	r := v.Rules
	return api.AdminCabalRules{
		JoinMode: string(r.JoinMode), VoterMode: string(r.VoterMode), Threshold: string(r.Threshold),
		ProposalExpirySeconds: int64(r.ProposalExpiry.Seconds()), SlippageBps: r.SlippageBps,
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
