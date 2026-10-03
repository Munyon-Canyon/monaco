package adapters

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Open      *app.OpenSessionHandler
	SetHandle *app.SetHandle
	Onboard   *app.Onboarding
	Update    app.UpdateProfileHandler
	Photo     app.UploadProfilePhotoHandler
	Reads     sqlc.DBTX
	Clock     clock.Clock
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

var _ httpx.IdentityRoutes = HTTP{}

func (h HTTP) PostOnboardingPhone(
	ctx context.Context, _ api.PostOnboardingPhoneRequestObject,
) (api.PostOnboardingPhoneResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	me, err := h.Onboard.LinkPhone(ctx, user)
	if err != nil {
		return nil, err
	}
	return api.PostOnboardingPhone200JSONResponse(wireMe(me)), nil
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

func (h HTTP) PostProfilePhoto(
	ctx context.Context, req api.PostProfilePhotoRequestObject,
) (api.PostProfilePhotoResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	contentType, ext, body, err := photo(req.Body)
	if err != nil {
		return nil, err
	}
	me, err := h.Photo.Handle(ctx, user, contentType, ext, body)
	if err != nil {
		return nil, err
	}
	return api.PostProfilePhoto200JSONResponse(wireMe(me)), nil
}

func photo(reader *multipart.Reader) (string, string, []byte, error) {
	const op = "identity.PostProfilePhoto"
	if reader == nil {
		return "", "", nil, errs.New(errs.CodePhotoInvalid, op)
	}
	part, err := reader.NextPart()
	if err != nil || part.FormName() != "photo" {
		return "", "", nil, errs.New(errs.CodePhotoInvalid, op)
	}
	body, err := io.ReadAll(part)
	if err != nil || len(body) == 0 {
		return "", "", nil, errs.New(errs.CodePhotoInvalid, op)
	}
	if _, err := reader.NextPart(); !errors.Is(err, io.EOF) {
		return "", "", nil, errs.New(errs.CodePhotoInvalid, op)
	}
	switch http.DetectContentType(body) {
	case "image/jpeg":
		return "image/jpeg", "jpg", body, nil
	case "image/png":
		return "image/png", "png", body, nil
	case "image/webp":
		return "image/webp", "webp", body, nil
	default:
		return "", "", nil, errs.New(errs.CodePhotoInvalid, op)
	}
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
