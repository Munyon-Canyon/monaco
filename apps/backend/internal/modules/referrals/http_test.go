package referrals_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/referralsapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/ratelimit"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type server struct {
	pool     *pgxpool.Pool
	handler  http.Handler
	raw      http.Handler
	verifier *auth.DevVerifier
	clock    *testkit.Clock
	logs     *testkit.Logs
}

func newServer(t *testing.T) server {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	cfg := config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "test-only"}}
	verifier, err := auth.NewDevVerifier(cfg, clk)
	if err != nil {
		t.Fatal(err)
	}
	policies, err := ratelimit.Load(openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	limiter, err := ratelimit.New(pool, clk, metricnoop.NewMeterProvider())
	if err != nil {
		t.Fatal(err)
	}
	logs := &testkit.Logs{}
	mount := referrals.New(module.Deps{Pool: pool, Clock: clk}).Mount
	h, err := httpx.Handler(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, logs),
		Tracer:       noop.NewTracerProvider(),
		Clock:        clk,
		IDs:          testkit.NewIDs(1),
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(pool, clk),
		Verifier:     verifier,
		RateLimit:    ratelimit.Middleware(limiter, policies, httpx.ActorKey, false),
		WebOrigins:   cfg.WebAllowedOrigins(),
	}, mount, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return server{
		pool: pool, handler: testkit.HTTP(t, h), raw: h, verifier: verifier, clock: clk, logs: logs,
	}
}

func (s server) get(t *testing.T, user ids.UserID) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/me/referral-code", nil)
	if user != (ids.UserID{}) {
		req.Header.Set("Authorization", "Bearer "+s.verifier.Mint(user.String(), s.clock.Now().Add(time.Hour)))
	}
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func TestGetMyReferralCode_returnsTheRandomLinkBeforeTheUnlock(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user := seedOwner(t, s.pool, owner{handle: "kaicenat", code: "k7m4qx2p"})
	rec := s.get(t, user)
	want := `{"code":"k7m4qx2p","link":"https://monacolabs.xyz/r/k7m4qx2p","handle_link":null,"handle_unlocked":false}`
	if rec.Code != http.StatusOK || !jsonEqual(t, rec.Body.Bytes(), want) {
		t.Fatalf("GET = %d %s, want 200 %s", rec.Code, rec.Body, want)
	}
}

func TestGetMyReferralCode_addsTheHandleLinkAfterTheUnlock(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user := seedOwner(t, s.pool, owner{handle: "kaicenat", code: "k7m4qx2p", unlocked: true})
	rec := s.get(t, user)
	want := `{"code":"k7m4qx2p","link":"https://monacolabs.xyz/r/k7m4qx2p",` +
		`"handle_link":"https://monacolabs.xyz/r/kaicenat","handle_unlocked":true}`
	if rec.Code != http.StatusOK || !jsonEqual(t, rec.Body.Bytes(), want) {
		t.Fatalf("GET = %d %s, want 200 %s", rec.Code, rec.Body, want)
	}
}

func TestGetMyReferralCode_hasNoHandleLinkForAnUnlockedUserWithoutAHandle(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user := seedOwner(t, s.pool, owner{code: "k7m4qx2p", unlocked: true})
	rec := s.get(t, user)
	want := `{"code":"k7m4qx2p","link":"https://monacolabs.xyz/r/k7m4qx2p","handle_link":null,"handle_unlocked":true}`
	if rec.Code != http.StatusOK || !jsonEqual(t, rec.Body.Bytes(), want) {
		t.Fatalf("GET = %d %s, want 200 %s", rec.Code, rec.Body, want)
	}
}

func TestGetMyReferralCode_answersARetryable503BeforeTheCodeIsMinted(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user := seedOwner(t, s.pool, owner{handle: "kaicenat"})
	rec := s.get(t, user)
	var p apibase.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable || p.Code != apibase.ReferralCodePending || !p.Retryable {
		t.Fatalf("GET = %d %s, want 503 referral_code_pending with retryable true", rec.Code, rec.Body)
	}
}

func TestGetMyReferralCode_refusesAMissingToken(t *testing.T) {
	t.Parallel()
	rec := newServer(t).get(t, ids.UserID{})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET without a token = %d %s, want 401", rec.Code, rec.Body)
	}
}

func TestMyCode_answersUserNotFoundWhenTheCodeOutlivesItsUser(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	ghost := testkit.NewIDs(7).NewV7()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO referral_codes (code, user_id, created_at) VALUES ('k7m4qx2p', $1, now())`, ghost,
	); err != nil {
		t.Fatal(err)
	}
	got, err := referrals.New(module.Deps{Pool: pool}).Resolver().MyCode(t.Context(), mustUser(t, ghost.String()))
	if errs.CodeOf(err) != errs.CodeUserNotFound || got != (app.MyCode{}) {
		t.Fatalf("MyCode = %+v, %v, want user_not_found", got, err)
	}
}

type brokenUsers struct{ identity.UserReader }

func (brokenUsers) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	return nil, errs.New(errs.CodeDBUnavailable, "test")
}

func TestMyCode_passesOnAFailedUserReadAndWrapsAFailedCodeRead(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := seedOwner(t, pool, owner{handle: "kaicenat", code: "k7m4qx2p"})
	_, err := app.Resolver{Reads: pool, Users: brokenUsers{}}.MyCode(t.Context(), user)
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("MyCode with a failing user read = %v, want db_unavailable", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = app.Resolver{Reads: pool, Users: brokenUsers{}}.MyCode(ctx, user)
	if errs.CodeOf(err) != errs.CodeInternal || !errors.Is(err, context.Canceled) {
		t.Fatalf("MyCode with a canceled context = %v, want internal wrapping context.Canceled", err)
	}
}

func TestHTTP_refusesCallersThatAreNotAUser(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{}
	for name, tc := range map[string]struct {
		actor *auth.Actor
		want  errs.Code
	}{
		"no actor":    {nil, errs.CodeUnauthorized},
		"agent":       {&auth.Actor{Kind: auth.ActorAgent, ID: "a1"}, errs.CodeForbidden},
		"bad user id": {&auth.Actor{Kind: auth.ActorUser, ID: "u1"}, errs.CodeUnauthorized},
	} {
		ctx := t.Context()
		if tc.actor != nil {
			ctx = auth.WithActor(ctx, *tc.actor)
		}
		if _, err := h.GetMyReferralCode(ctx, api.GetMyReferralCodeRequestObject{}); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: GetMyReferralCode err = %v, want %s", name, err, tc.want)
		}
	}
}

func TestPostMeReferral_refusesCallersAndBodiesItCannotServe(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{}
	user := auth.Actor{Kind: auth.ActorUser, ID: testkit.NewIDs(3).NewV7().String()}
	body := func(code string, source api.ReferralSource) *api.PostMeReferralJSONRequestBody {
		return &api.PostMeReferralJSONRequestBody{Code: code, Source: source}
	}
	for name, tc := range map[string]struct {
		actor *auth.Actor
		body  *api.PostMeReferralJSONRequestBody
		want  errs.Code
	}{
		"no actor":       {nil, body("k7m4qx2p", api.Manual), errs.CodeUnauthorized},
		"no body":        {&user, nil, errs.CodeInvalidInput},
		"unknown source": {&user, body("k7m4qx2p", "carrier_pigeon"), errs.CodeInvalidInput},
		"empty code":     {&user, body("", api.Manual), errs.CodeInvalidInput},
	} {
		ctx := t.Context()
		if tc.actor != nil {
			ctx = auth.WithActor(ctx, *tc.actor)
		}
		req := api.PostMeReferralRequestObject{Body: tc.body}
		if _, err := h.PostMeReferral(ctx, req); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: PostMeReferral err = %v, want %s", name, err, tc.want)
		}
	}
}

type countedUsers struct {
	identity.UserReader
	calls  *int
	failAt int
	missAt int
}

func (c countedUsers) UsersByID(ctx context.Context, in []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	*c.calls++
	switch *c.calls {
	case c.failAt:
		return nil, errs.New(errs.CodeDBUnavailable, "test")
	case c.missAt:
		return map[ids.UserID]identity.UserCard{}, nil
	}
	return c.UserReader.UsersByID(ctx, in)
}

func TestPostMeReferral_passesOnAReadThatFailsAfterTheAttach(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		failAt, missAt int
		want           errs.Code
	}{
		"resolve fails":  {failAt: 3, want: errs.CodeDBUnavailable},
		"referrer fails": {failAt: 4, want: errs.CodeDBUnavailable},
		"referrer gone":  {missAt: 4, want: errs.CodeInternal},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool := testkit.DB(t)
			clock := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
			generator := testkit.NewIDs(95)
			referrer, caller := mustUser(t, generator.NewV7().String()), mustUser(t, generator.NewV7().String())
			if _, err := pool.Exec(
				t.Context(),
				`INSERT INTO referral_codes (code, user_id, created_at) VALUES ('k7m4qx2p', $1, now())`,
				referrer.UUID(),
			); err != nil {
				t.Fatal(err)
			}
			calls := 0
			users := countedUsers{
				UserReader: fakes.NewIdentity([]identity.UserCard{
					{ID: referrer, AccountStatus: identity.AccountActive},
					{ID: caller, AccountStatus: identity.AccountActive, CreatedAt: clock.Now()},
				}, nil),
				calls: &calls, failAt: tc.failAt, missAt: tc.missAt,
			}
			resolver := app.Resolver{Reads: pool, Users: users}
			h := adapters.HTTP{
				Codes: resolver,
				Attach: app.NewAttachReferralHandler(app.AttachReferralDeps{
					UoW: db.New(pool, generator, clock), Resolver: resolver, Users: users, IDs: generator, Clock: clock,
				}),
			}
			ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: caller.String()})
			req := api.PostMeReferralRequestObject{
				Body: &api.PostMeReferralJSONRequestBody{Code: "k7m4qx2p", Source: api.Manual},
			}
			if _, err := h.PostMeReferral(ctx, req); errs.CodeOf(err) != tc.want {
				t.Fatalf("PostMeReferral err = %v, want %s", err, tc.want)
			}
		})
	}
}

func mustUser(t *testing.T, raw string) ids.UserID {
	t.Helper()
	id, err := ids.ParseUserID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func jsonEqual(t *testing.T, got []byte, want string) bool {
	t.Helper()
	var a, b map[string]any
	if err := json.Unmarshal(got, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		return false
	}
	for k, v := range b {
		if a[k] != v {
			return false
		}
	}
	return true
}
