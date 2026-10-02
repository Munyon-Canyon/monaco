package referrals_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type server struct {
	pool     *pgxpool.Pool
	handler  http.Handler
	verifier *auth.DevVerifier
	clock    *testkit.Clock
}

func newServer(t *testing.T) server {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	verifier, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "test-only"}}, clk)
	if err != nil {
		t.Fatal(err)
	}
	var routes httpx.Routes
	referrals.New(module.Deps{Pool: pool}).Routes(&routes)
	h, err := httpx.Handler(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer:       noop.NewTracerProvider(),
		Clock:        clk,
		IDs:          testkit.NewIDs(1),
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(pool, clk),
		Verifier:     verifier,
	}, routes, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return server{pool: pool, handler: testkit.HTTP(t, h), verifier: verifier, clock: clk}
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
	var p api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable || p.Code != api.ReferralCodePending || !p.Retryable {
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
