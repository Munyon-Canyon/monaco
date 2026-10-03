package adapters

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Create  *app.CreateCabalHandler
	Join    *app.JoinCabalHandler
	Request *app.RequestAccessHandler
	Revoke  *app.RevokeAccessHandler
	Decide  *app.DecideAccessHandler
	Update  *app.UpdateCabalHandler
	Picture *app.SetCabalPictureHandler
	DB      sqlc.DBTX
	Users   app.UserCards
}

var (
	_ httpx.CabalRoutes        = HTTP{}
	_ httpx.CabalJoinRoutes    = HTTP{}
	_ httpx.CabalAccessRoutes  = HTTP{}
	_ httpx.CabalPictureRoutes = HTTP{}
)

func (h HTTP) PostCabal(
	ctx context.Context, req api.PostCabalRequestObject,
) (api.PostCabalResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	cmd, err := createCommand(user, req)
	if err != nil {
		return nil, err
	}
	created, err := h.Create.Handle(ctx, cmd)
	if err != nil {
		return nil, err
	}
	view, err := app.GetCabal(ctx, h.DB, h.Users, created.ID, user)
	if err != nil {
		return nil, err
	}
	return api.PostCabal201JSONResponse(wireCabal(view)), nil
}

const (
	defaultCabalSearchLimit = 20
	maxCabalSearchLimit     = 50
)

type cabalSearchCursor struct {
	MemberCount int32  `json:"member_count"`
	CreatedAt   string `json:"created_at"`
	ID          string `json:"id"`
}

func (h HTTP) GetCabals(
	ctx context.Context, req api.GetCabalsRequestObject,
) (api.GetCabalsResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	query := ""
	if req.Params.Query != nil {
		query = strings.TrimSpace(*req.Params.Query)
	}
	if query != "" && utf8.RuneCountInString(query) < 2 {
		return nil, errs.New(errs.CodeInvalidInput, "cabal.GetCabals", slog.String("reason", "query"))
	}
	limit := defaultCabalSearchLimit
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	if limit < 1 || limit > maxCabalSearchLimit {
		return nil, errs.New(errs.CodeInvalidInput, "cabal.GetCabals", slog.String("reason", "limit"))
	}
	params, err := cabalSearchParams(user, query, limit, req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	rows, err := sqlc.New(h.DB).SearchCabals(ctx, params)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "cabal.GetCabals")
	}
	next := nextCabalCursor(rows, limit)
	if len(rows) > limit {
		rows = rows[:limit]
	}
	items := make([]api.CabalSearchItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, api.CabalSearchItem{
			Id: row.ID, Name: row.Name, PictureUrl: nullableText(row.PictureUrl),
			MemberCount: row.MemberCount, JoinMode: row.JoinMode, IsMember: row.IsMember,
			MyAccessRequestStatus: nullableText(row.MyAccessRequestStatus),
		})
	}
	return api.GetCabals200JSONResponse(api.CabalSearchPage{Items: items, NextCursor: next}), nil
}

func (h HTTP) GetMyCabals(
	ctx context.Context, _ api.GetMyCabalsRequestObject,
) (api.GetMyCabalsResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := sqlc.New(h.DB).ListMyCabals(ctx, user.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "cabal.GetMyCabals")
	}
	items := make([]api.MyCabal, 0, len(rows))
	for _, row := range rows {
		items = append(items, api.MyCabal{
			Id: row.ID, Name: row.Name, PictureUrl: nullableText(row.PictureUrl), Role: row.Role,
			CanVote: row.CanVote, MemberCount: row.MemberCount, JoinedAt: row.JoinedAt,
			PendingRequestCount: row.PendingRequestCount,
		})
	}
	return api.GetMyCabals200JSONResponse(items), nil
}

func cabalSearchParams(
	user ids.UserID, query string, limit int, rawCursor *string,
) (sqlc.SearchCabalsParams, error) {
	pageSize := int32(1)
	for range limit {
		pageSize++
	}
	params := sqlc.SearchCabalsParams{ActorID: user.UUID(), Query: query, PageSize: pageSize}
	if rawCursor == nil {
		return params, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(*rawCursor)
	if err != nil {
		return sqlc.SearchCabalsParams{}, invalidCabalCursor()
	}
	var cursor cabalSearchCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return sqlc.SearchCabalsParams{}, invalidCabalCursor()
	}
	createdAt, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
	if err != nil {
		return sqlc.SearchCabalsParams{}, invalidCabalCursor()
	}
	id, err := uuid.Parse(cursor.ID)
	if err != nil || cursor.MemberCount < 1 {
		return sqlc.SearchCabalsParams{}, invalidCabalCursor()
	}
	params.CursorMemberCount = pgtype.Int4{Int32: cursor.MemberCount, Valid: true}
	params.CursorCreatedAt = pgtype.Timestamptz{Time: createdAt, Valid: true}
	params.CursorID = pgtype.UUID{Bytes: id, Valid: true}
	return params, nil
}

func invalidCabalCursor() error {
	return errs.New(errs.CodeInvalidInput, "cabal.GetCabals", slog.String("reason", "cursor"))
}

func nextCabalCursor(rows []sqlc.SearchCabalsRow, limit int) *string {
	if len(rows) <= limit {
		return nil
	}
	last := rows[limit-1]
	cursor := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(
		`{"member_count":%d,"created_at":%q,"id":%q}`,
		last.MemberCount, last.CreatedAt.UTC().Format(time.RFC3339Nano), last.ID.String(),
	)))
	return &cursor
}

func nullableText(text pgtype.Text) *string {
	if !text.Valid {
		return nil
	}
	return &text.String
}

func (h HTTP) GetCabal(
	ctx context.Context, req api.GetCabalRequestObject,
) (api.GetCabalResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	view, err := app.GetCabal(ctx, h.DB, h.Users, ids.CabalIDFrom(req.Id), user)
	if err != nil {
		return nil, err
	}
	return api.GetCabal200JSONResponse(wireCabal(view)), nil
}

func (h HTTP) PatchCabal(
	ctx context.Context, req api.PatchCabalRequestObject,
) (api.PatchCabalResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	cmd, err := updateCommand(user, req)
	if err != nil {
		return nil, err
	}
	if err := h.Update.Handle(ctx, cmd); err != nil {
		return nil, err
	}
	view, err := app.GetCabal(ctx, h.DB, h.Users, cmd.CabalID, user)
	if err != nil {
		return nil, err
	}
	return api.PatchCabal200JSONResponse(wireCabal(view)), nil
}

func updateCommand(user ids.UserID, req api.PatchCabalRequestObject) (app.UpdateCabal, error) {
	if req.Body == nil {
		return app.UpdateCabal{}, errs.New(errs.CodeInvalidInput, "cabal.PatchCabal", slog.String("reason", "body"))
	}
	body := req.Body
	cmd := app.UpdateCabal{
		ActorID: user, CabalID: ids.CabalIDFrom(req.Id),
		Rules: domain.RulesPatch{
			JoinMode: body.JoinMode, VoterMode: body.VoterMode, Threshold: body.Threshold,
			ExpirySeconds: body.ProposalExpirySeconds, SlippageBps: body.SlippageBps,
		},
	}
	if body.Name != nil {
		name, err := domain.ParseName(*body.Name)
		if err != nil {
			return app.UpdateCabal{}, err
		}
		cmd.Name = &name
	}
	if body.VoterIds != nil {
		voters := make([]ids.UserID, 0, len(*body.VoterIds))
		for _, voter := range *body.VoterIds {
			voters = append(voters, ids.UserIDFrom(voter))
		}
		cmd.VoterIDs = &voters
	}
	return cmd, nil
}

func (h HTTP) PutCabalPicture(
	ctx context.Context, req api.PutCabalPictureRequestObject,
) (api.PutCabalPictureResponseObject, error) {
	image, err := httpx.ReadImage(req.Body, "picture", "cabal.PutCabalPicture")
	if err != nil {
		return nil, err
	}
	view, err := h.setPicture(ctx, ids.CabalIDFrom(req.Id), &app.Picture{
		ContentType: image.ContentType, Ext: image.Ext, Body: image.Body,
	})
	if err != nil {
		return nil, err
	}
	return api.PutCabalPicture200JSONResponse(view), nil
}

func (h HTTP) DeleteCabalPicture(
	ctx context.Context, req api.DeleteCabalPictureRequestObject,
) (api.DeleteCabalPictureResponseObject, error) {
	view, err := h.setPicture(ctx, ids.CabalIDFrom(req.Id), nil)
	if err != nil {
		return nil, err
	}
	return api.DeleteCabalPicture200JSONResponse(view), nil
}

func (h HTTP) setPicture(ctx context.Context, cabalID ids.CabalID, picture *app.Picture) (api.Cabal, error) {
	user, err := caller(ctx)
	if err != nil {
		return api.Cabal{}, err
	}
	if err := h.Picture.Handle(
		ctx,
		app.SetCabalPicture{ActorID: user, CabalID: cabalID, Picture: picture},
	); err != nil {
		return api.Cabal{}, err
	}
	view, err := app.GetCabal(ctx, h.DB, h.Users, cabalID, user)
	if err != nil {
		return api.Cabal{}, err
	}
	return wireCabal(view), nil
}

func (h HTTP) PostCabalMember(
	ctx context.Context, req api.PostCabalMemberRequestObject,
) (api.PostCabalMemberResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	cabalID := ids.CabalIDFrom(req.Id)
	if err := h.Join.Handle(ctx, app.JoinCabal{ActorID: user, CabalID: cabalID}); err != nil {
		return nil, err
	}
	view, err := app.GetCabal(ctx, h.DB, h.Users, cabalID, user)
	if err != nil {
		return nil, err
	}
	return api.PostCabalMember200JSONResponse(wireCabal(view)), nil
}

func (h HTTP) GetCabalByCode(
	ctx context.Context, req api.GetCabalByCodeRequestObject,
) (api.GetCabalByCodeResponseObject, error) {
	const op = "cabal.GetCabalByCode"
	if _, err := caller(ctx); err != nil {
		return nil, err
	}
	row, err := sqlc.New(h.DB).FindCabalByInviteCode(ctx, strings.ToUpper(strings.TrimSpace(req.Code)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errs.New(errs.CodeCabalNotFound, op)
	}
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	return api.GetCabalByCode200JSONResponse(api.CabalPreview{
		Id: row.ID, Name: row.Name, PictureUrl: nullableText(row.PictureUrl), JoinMode: row.JoinMode,
		MemberCount: row.MemberCount,
	}), nil
}

func (h HTTP) PostCabalAccessRequest(
	ctx context.Context, req api.PostCabalAccessRequestRequestObject,
) (api.PostCabalAccessRequestResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	filed, err := h.Request.Handle(ctx, app.RequestAccess{ActorID: user, CabalID: ids.CabalIDFrom(req.Id)})
	if err != nil {
		return nil, err
	}
	return api.PostCabalAccessRequest201JSONResponse(wireAccessValue(filed)), nil
}

func (h HTTP) DeleteCabalAccessRequest(
	ctx context.Context, req api.DeleteCabalAccessRequestRequestObject,
) (api.DeleteCabalAccessRequestResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	revoked, err := h.Revoke.Handle(ctx, app.RevokeAccess{
		ActorID: user, CabalID: ids.CabalIDFrom(req.Id), RequestID: ids.AccessRequestIDFrom(req.RequestId),
	})
	if err != nil {
		return nil, err
	}
	return api.DeleteCabalAccessRequest200JSONResponse(wireAccessValue(revoked)), nil
}

func (h HTTP) PostCabalAccessDecision(
	ctx context.Context, req api.PostCabalAccessDecisionRequestObject,
) (api.PostCabalAccessDecisionResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, errs.New(errs.CodeInvalidInput, "cabal.PostCabalAccessDecision", slog.String("reason", "body"))
	}
	decided, err := h.Decide.Handle(ctx, app.DecideAccess{
		ActorID: user, CabalID: ids.CabalIDFrom(req.Id), RequestID: ids.AccessRequestIDFrom(req.RequestId),
		Decision: app.Decision(req.Body.Decision),
	})
	if err != nil {
		return nil, err
	}
	return api.PostCabalAccessDecision200JSONResponse(wireAccessValue(decided)), nil
}

func (h HTTP) GetCabalAccessRequests(
	ctx context.Context, req api.GetCabalAccessRequestsRequestObject,
) (api.GetCabalAccessRequestsResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	pending, err := app.ListAccessRequests(ctx, h.DB, h.Users, ids.CabalIDFrom(req.Id), user)
	if err != nil {
		return nil, err
	}
	items := make([]api.CabalAccessRequest, 0, len(pending))
	for _, p := range pending {
		items = append(items, api.CabalAccessRequest{Id: p.ID, User: wirePerson(p.User), CreatedAt: p.CreatedAt})
	}
	return api.GetCabalAccessRequests200JSONResponse(items), nil
}

func createCommand(user ids.UserID, req api.PostCabalRequestObject) (app.CreateCabal, error) {
	const op = "cabal.PostCabal"
	if req.Body == nil {
		return app.CreateCabal{}, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "body"))
	}
	name, err := domain.ParseName(req.Body.Name)
	if err != nil {
		return app.CreateCabal{}, err
	}
	slippage := domain.DefaultSlippageBps
	if req.Body.SlippageBps != nil {
		slippage = *req.Body.SlippageBps
	}
	rules, err := domain.NewRules(
		req.Body.JoinMode, req.Body.VoterMode, req.Body.Threshold, req.Body.ProposalExpirySeconds, slippage,
	)
	if err != nil {
		return app.CreateCabal{}, err
	}
	return app.CreateCabal{
		ActorID: user, IdempotencyKey: req.Params.IdempotencyKey, Name: name, Rules: rules,
	}, nil
}

func caller(ctx context.Context) (ids.UserID, error) {
	const op = "cabal.caller"
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

func wireCabal(view app.CabalView) api.Cabal {
	members := make([]api.CabalMember, 0, len(view.Members))
	for _, member := range view.Members {
		members = append(members, api.CabalMember{
			UserId: member.UserID, Handle: nullString(member.Handle), DisplayName: member.DisplayName,
			PhotoUrl: nullString(member.PhotoURL), Role: member.Role, CanVote: member.CanVote,
			JoinedAt: member.JoinedAt,
		})
	}
	return api.Cabal{
		Id: view.ID.UUID(), Name: view.Name, PictureUrl: view.PictureURL, Status: view.Status,
		Rules: api.CabalRules{
			JoinMode: view.JoinMode, VoterMode: view.VoterMode, Threshold: view.Threshold,
			ProposalExpirySeconds: view.ExpirySeconds, SlippageBps: view.SlippageBps,
		},
		Creator:     wirePerson(view.Creator),
		MemberCount: view.MemberCount, Members: members, Me: wireMe(view.Me),
		MyAccessRequest: wireAccess(view.Access), InviteCode: view.InviteCode,
		TreasuryAddress: string(view.TreasuryAddress),
	}
}

func wirePerson(p app.Person) api.CabalPerson {
	return api.CabalPerson{
		UserId: p.UserID, Handle: nullString(p.Handle), DisplayName: p.DisplayName, PhotoUrl: nullString(p.PhotoURL),
	}
}

func wireMe(me *app.Membership) *api.CabalMembership {
	if me == nil {
		return nil
	}
	return &api.CabalMembership{Role: me.Role, CanVote: me.CanVote}
}

func wireAccess(access *app.Access) *api.CabalAccess {
	if access == nil {
		return nil
	}
	wired := wireAccessValue(*access)
	return &wired
}

func wireAccessValue(access app.Access) api.CabalAccess {
	return api.CabalAccess{Id: access.ID, Direction: access.Direction, Status: access.Status}
}

func nullString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
