package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/identityapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Open      *app.OpenSessionHandler
	SetHandle *app.SetHandle
	Onboard   *app.Onboarding
	Update    app.UpdateProfileHandler
	Photo     app.UploadProfilePhotoHandler
	Delete    *app.DeleteAccount
	DevX      *app.DevXLink
	Reads     sqlc.DBTX
	Clock     clock.Clock
	Cards     app.UserCards
	Follows   app.FollowCounts
}

func (h HTTP) PutMeHandle(
	ctx context.Context, req api.PutMeHandleRequestObject,
) (api.PutMeHandleResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	me, err := h.SetHandle.Handle(ctx, user, req.Body.Handle, h.Clock.Now())
	if err != nil {
		return nil, err
	}
	return api.PutMeHandle200JSONResponse(wireMe(me)), nil
}

var _ api.StrictServerInterface = HTTP{}

func (h HTTP) PostOnboardingPhone(
	ctx context.Context, _ api.PostOnboardingPhoneRequestObject,
) (api.PostOnboardingPhoneResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	me, err := h.Onboard.LinkPhone(ctx, app.LinkPhone{UserID: user})
	if err != nil {
		return nil, err
	}
	return api.PostOnboardingPhone200JSONResponse(wireMe(me)), nil
}

func (h HTTP) PostOnboardingSocials(
	ctx context.Context, _ api.PostOnboardingSocialsRequestObject,
) (api.PostOnboardingSocialsResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	me, err := h.Onboard.LinkSocials(ctx, app.LinkSocials{UserID: user})
	if err != nil {
		return nil, err
	}
	return api.PostOnboardingSocials200JSONResponse(wireMe(me)), nil
}

func (h HTTP) PostOnboardingSkip(
	ctx context.Context, req api.PostOnboardingSkipRequestObject,
) (api.PostOnboardingSkipResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, errs.New(errs.CodeInvalidInput, "identity.PostOnboardingSkip")
	}
	me, err := h.Onboard.Skip(ctx, app.SkipOnboardingStep{UserID: user, Step: domain.OnboardingStep(req.Body.Step)})
	if err != nil {
		return nil, err
	}
	return api.PostOnboardingSkip200JSONResponse(wireMe(me)), nil
}

func (h HTTP) PostAuthSession(
	ctx context.Context, req api.PostAuthSessionRequestObject,
) (api.PostAuthSessionResponseObject, error) {
	header := ""
	if req.Params.Authorization != nil {
		header = *req.Params.Authorization
	}
	token, ok := httpx.BearerToken(header)
	if !ok {
		return nil, errs.New(
			errs.CodeUnauthorized,
			"identity.PostAuthSession",
			slog.String("reason", "no_bearer_token"),
		)
	}
	me, err := h.Open.Handle(ctx, app.OpenSession{Token: token})
	if err != nil {
		return nil, err
	}
	return api.PostAuthSession200JSONResponse(wireMe(me)), nil
}

func (h HTTP) GetHandleAvailability(
	ctx context.Context, req api.GetHandleAvailabilityRequestObject,
) (api.GetHandleAvailabilityResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	got, err := app.HandleAvailability(ctx, h.Reads, user, req.Handle, h.Clock.Now())
	if err != nil {
		return nil, err
	}
	return api.GetHandleAvailability200JSONResponse(wireAvailability(got)), nil
}

func (h HTTP) GetMe(ctx context.Context, _ api.GetMeRequestObject) (api.GetMeResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	me, err := app.GetMe(ctx, h.Reads, user)
	if err != nil {
		return nil, err
	}
	return api.GetMe200JSONResponse(wireMe(me)), nil
}

func (h HTTP) GetUser(ctx context.Context, req api.GetUserRequestObject) (api.GetUserResponseObject, error) {
	viewer, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	user, err := app.GetUser(ctx, h.Cards, h.Follows, viewer, ids.UserIDFrom(req.Id))
	if err != nil {
		return nil, err
	}
	return api.GetUser200JSONResponse{
		Id: user.ID.UUID(), Handle: user.Handle, DisplayName: user.DisplayName, PhotoUrl: present(user.PhotoURL),
		FollowerCount: user.FollowerCount, FollowingCount: user.FollowingCount, FollowedByMe: user.FollowedByMe,
		BlockedByMe: user.BlockedByMe,
	}, nil
}

func (h HTTP) SearchUsers(
	ctx context.Context, req api.SearchUsersRequestObject,
) (api.SearchUsersResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	users, err := app.SearchUsers(ctx, h.Reads, user, req.Params.Query)
	if err != nil {
		return nil, err
	}
	out := make([]api.UserSummary, len(users))
	for i, found := range users {
		out[i] = api.UserSummary{
			UserId:      found.ID.UUID(),
			Handle:      found.Handle,
			DisplayName: found.DisplayName,
			PhotoUrl:    present(found.PhotoURL),
		}
	}
	return api.SearchUsers200JSONResponse{Users: out}, nil
}

func (h HTTP) PatchMe(ctx context.Context, req api.PatchMeRequestObject) (api.PatchMeResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, errs.New(errs.CodeInvalidInput, "identity.PatchMe")
	}
	me, err := h.Update.Handle(ctx, user, req.Body.DisplayName)
	if err != nil {
		return nil, err
	}
	return api.PatchMe200JSONResponse(wireMe(me)), nil
}

func (h HTTP) DeleteMe(ctx context.Context, _ api.DeleteMeRequestObject) (api.DeleteMeResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.Delete.Handle(ctx, user); err != nil {
		return nil, err
	}
	return api.DeleteMe204Response{}, nil
}

func (h HTTP) PostProfilePhoto(
	ctx context.Context, req api.PostProfilePhotoRequestObject,
) (api.PostProfilePhotoResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	image, err := httpx.ReadImage(req.Body, "photo", "identity.PostProfilePhoto")
	if err != nil {
		return nil, err
	}
	me, err := h.Photo.Handle(ctx, user, image.ContentType, image.Ext, image.Body)
	if err != nil {
		return nil, err
	}
	return api.PostProfilePhoto200JSONResponse(wireMe(me)), nil
}

func caller(ctx context.Context) (ids.UserID, error) {
	const op = "identity.caller"
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

func wireMe(m app.Me) api.Me {
	return api.Me{
		Id: m.ID.UUID(), Handle: present(m.Handle), DisplayName: m.DisplayName, PhotoUrl: present(m.PhotoURL),
		AuthState: api.AuthState(m.AuthState), AccountStatus: api.AccountStatus(m.AccountStatus),
		LoginProvider:       api.LoginProvider(m.LoginProvider),
		MemberWalletAddress: string(m.MemberWalletAddress), PhoneLinked: m.PhoneLinked,
		XUsername: present(m.XUsername), HandleChangeableAt: m.HandleChangeableAt, CreatedAt: m.CreatedAt,
	}
}

func wireAvailability(a app.Availability) api.HandleAvailability {
	out := api.HandleAvailability{Handle: a.Handle, Available: a.Available}
	if a.Reason == "" {
		return out
	}
	reason := api.HandleAvailabilityReason(a.Reason)
	out.Reason = &reason
	return out
}

func present(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (h HTTP) PostDevXLink(
	ctx context.Context, req api.PostDevXLinkRequestObject,
) (api.PostDevXLinkResponseObject, error) {
	if h.DevX == nil {
		return nil, errs.New(errs.CodeNotFound, "identity.PostDevXLink")
	}
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	username := ""
	if req.Body != nil && req.Body.Username != nil {
		username = *req.Body.Username
	}
	if err := h.DevX.Link(ctx, user, username); err != nil {
		return nil, err
	}
	return api.PostDevXLink204Response{}, nil
}

func (h HTTP) DeleteDevXLink(
	ctx context.Context, _ api.DeleteDevXLinkRequestObject,
) (api.DeleteDevXLinkResponseObject, error) {
	if h.DevX == nil {
		return nil, errs.New(errs.CodeNotFound, "identity.DeleteDevXLink")
	}
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.DevX.Unlink(ctx, user); err != nil {
		return nil, err
	}
	return api.DeleteDevXLink204Response{}, nil
}
