package httpx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
)

func Handler(d Deps, ssi api.StrictServerInterface, spec []byte) (http.Handler, error) {
	c, err := LoadContract(spec)
	if err != nil {
		return nil, err
	}
	return HandlerFor(d, ssi, c)
}

func HandlerFor(d Deps, ssi api.StrictServerInterface, c *Contract) (http.Handler, error) {
	return handler(d, ssi, c, nil)
}

func handler(
	d Deps, ssi api.StrictServerInterface, c *Contract, mws []api.StrictMiddlewareFunc,
) (http.Handler, error) {
	if d.Idempotency == nil || d.Verifier == nil {
		return nil, errs.New(
			errs.CodeInternal,
			"httpx.Handler",
			slog.String("missing", "Deps.Idempotency or Deps.Verifier"),
		)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		Problem(w, r, errs.New(errs.CodeNotFound, "httpx.route"))
	})
	strict := api.NewStrictHandlerWithOptions(ssi, mws, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  invalidRequest,
		ResponseErrorHandlerFunc: Problem,
	})
	api.HandlerWithOptions(strict, api.StdHTTPServerOptions{
		BaseRouter:       mux,
		ErrorHandlerFunc: invalidRequest,
		Middlewares:      middlewares(d, c),
	})
	return d.wrapContract(mux), nil
}

func middlewares(d Deps, c *Contract) []api.MiddlewareFunc {
	mws := []api.MiddlewareFunc{Idempotency(d.Idempotency), c.validate}
	if d.RateLimit != nil {
		mws = append(mws, d.RateLimit)
	}
	return append(mws, Auth(d.Verifier), c.limit(d.MaxBodyBytes), c.resolve)
}

func invalidRequest(w http.ResponseWriter, r *http.Request, err error) {
	Problem(w, r, errs.Wrap(err, errs.CodeInvalidInput, "httpx.decodeRequest"))
}

func NewServer(h http.Handler, t config.Timeouts) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: t.HTTPServerRead,
		ReadTimeout:       t.HTTPServerRead,
		WriteTimeout:      t.HTTPServerWrite,
	}
}

func Serve(ctx context.Context, ln net.Listener, srv *http.Server, shutdownTimeout time.Duration) error {
	shutdown := make(chan error, 1)
	stop := context.AfterFunc(ctx, func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		shutdown <- srv.Shutdown(shutdownCtx)
	})
	defer stop()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	if err := <-shutdown; err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

type Routes struct {
	Health
	sse.Stream
	GovernanceRoutes
	IdentityRoutes
	NotifyRoutes
	ReferralsRoutes
	SocialRoutes
	SystemRoutes
	CabalRoutes
	CabalJoinRoutes
	CabalAccessRoutes
	MarketRoutes
}

type CabalAccessRoutes interface {
	PostCabalAccessRequest(
		context.Context, api.PostCabalAccessRequestRequestObject,
	) (api.PostCabalAccessRequestResponseObject, error)
	GetCabalAccessRequests(
		context.Context, api.GetCabalAccessRequestsRequestObject,
	) (api.GetCabalAccessRequestsResponseObject, error)
	DeleteCabalAccessRequest(
		context.Context, api.DeleteCabalAccessRequestRequestObject,
	) (api.DeleteCabalAccessRequestResponseObject, error)
	PostCabalAccessDecision(
		context.Context, api.PostCabalAccessDecisionRequestObject,
	) (api.PostCabalAccessDecisionResponseObject, error)
}

type CabalJoinRoutes interface {
	PostCabalMember(context.Context, api.PostCabalMemberRequestObject) (api.PostCabalMemberResponseObject, error)
	GetCabalByCode(context.Context, api.GetCabalByCodeRequestObject) (api.GetCabalByCodeResponseObject, error)
}

type GovernanceRoutes interface {
	PostProposalVote(context.Context, api.PostProposalVoteRequestObject) (api.PostProposalVoteResponseObject, error)
}

type IdentityRoutes interface {
	IdentitySessionRoutes
	IdentityProfileRoutes
	IdentityHandleRoutes
}

type IdentitySessionRoutes interface {
	PostAuthSession(context.Context, api.PostAuthSessionRequestObject) (api.PostAuthSessionResponseObject, error)
	GetMe(context.Context, api.GetMeRequestObject) (api.GetMeResponseObject, error)
}

type IdentityProfileRoutes interface {
	PatchMe(context.Context, api.PatchMeRequestObject) (api.PatchMeResponseObject, error)
	PostProfilePhoto(context.Context, api.PostProfilePhotoRequestObject) (api.PostProfilePhotoResponseObject, error)
}

type IdentityHandleRoutes interface {
	GetHandleAvailability(
		context.Context, api.GetHandleAvailabilityRequestObject,
	) (api.GetHandleAvailabilityResponseObject, error)
	PutMeHandle(context.Context, api.PutMeHandleRequestObject) (api.PutMeHandleResponseObject, error)
}

type NotifyRoutes interface {
	PostDevice(context.Context, api.PostDeviceRequestObject) (api.PostDeviceResponseObject, error)
	DeleteDevice(context.Context, api.DeleteDeviceRequestObject) (api.DeleteDeviceResponseObject, error)
}

type ReferralsRoutes interface {
	GetMyReferralCode(context.Context, api.GetMyReferralCodeRequestObject) (api.GetMyReferralCodeResponseObject, error)
}

type SocialRoutes interface {
	PostUserFollow(context.Context, api.PostUserFollowRequestObject) (api.PostUserFollowResponseObject, error)
	DeleteUserFollow(context.Context, api.DeleteUserFollowRequestObject) (api.DeleteUserFollowResponseObject, error)
}

type SystemRoutes interface {
	PostSystemPing(context.Context, api.PostSystemPingRequestObject) (api.PostSystemPingResponseObject, error)
	GetSystemPing(context.Context, api.GetSystemPingRequestObject) (api.GetSystemPingResponseObject, error)
}

type CabalRoutes interface {
	PostCabal(context.Context, api.PostCabalRequestObject) (api.PostCabalResponseObject, error)
	GetCabals(context.Context, api.GetCabalsRequestObject) (api.GetCabalsResponseObject, error)
	GetCabal(context.Context, api.GetCabalRequestObject) (api.GetCabalResponseObject, error)
	GetMyCabals(context.Context, api.GetMyCabalsRequestObject) (api.GetMyCabalsResponseObject, error)
}

type MarketRoutes interface {
	GetAssets(context.Context, api.GetAssetsRequestObject) (api.GetAssetsResponseObject, error)
	GetAsset(context.Context, api.GetAssetRequestObject) (api.GetAssetResponseObject, error)
	GetAssetChart(context.Context, api.GetAssetChartRequestObject) (api.GetAssetChartResponseObject, error)
}

var _ api.StrictServerInterface = Routes{}

type Health struct{}

func (Health) GetHealthz(context.Context, api.GetHealthzRequestObject) (api.GetHealthzResponseObject, error) {
	return api.GetHealthz200TextResponse("ok\n"), nil
}
