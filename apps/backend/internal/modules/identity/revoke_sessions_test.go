package identity_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	identityapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/identityapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

type revokeFixture struct {
	verifierFixture
	handler http.Handler
	user    testkit.SeededUser
	calls   int
}

func newRevokeFixture(t *testing.T, status string) *revokeFixture {
	t.Helper()
	f := &revokeFixture{verifierFixture: newVerifierFixture(t)}
	f.user = testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "revoker", WithWallet: true, AccountStatus: status})
	mount := identity.New(
		module.Deps{
			Pool:  f.pool,
			UoW:   db.New(f.pool, testkit.NewIDs(9), f.clock),
			IDs:   testkit.NewIDs(10),
			Clock: f.clock,
		},
		identity.WithPrivy(&privyfake.Users{}, &privyfake.Wallets{}),
		identity.WithHints(&recordedHints{}),
		identity.WithHoldings(fakes.NewTreasury(), fakes.NewBalances()),
	).Mount
	h, err := httpx.HandlerFor(httpx.Deps{
		Logger: observability.NewLogger(config.Config{Env: config.EnvTest}, &testkit.Logs{}),
		Tracer: tracenoop.NewTracerProvider(), Clock: f.clock, IDs: testkit.NewIDs(11), MaxBodyBytes: 1 << 20,
		Idempotency: db.NewIdempotencyStore(f.pool, f.clock), Verifier: f.verifier(t, config.EnvTest),
	}, mount, specContract(t))
	if err != nil {
		t.Fatal(err)
	}
	f.handler = h
	return f
}

func (f *revokeFixture) token(sid string, issued time.Time) string {
	claims, _ := json.Marshal(map[string]any{
		"iss": "privy.io", "aud": privyAppID, "sub": f.user.PrivyUserID, "sid": sid,
		"iat": issued.Unix(), "exp": issued.Add(time.Hour).Unix(),
	})
	return fakes.SignES256(fakes.PrivyTokenKey(), es256JWT, string(claims))
}

func (f *revokeFixture) do(t *testing.T, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	f.calls++
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "revoke-"+strconv.Itoa(f.calls))
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func (f *revokeFixture) revoke(t *testing.T, token, scope string) *httptest.ResponseRecorder {
	t.Helper()
	return f.do(t, token, http.MethodPost, "/v1/me/sessions/revoke", `{"scope":"`+scope+`"}`)
}

func (f *revokeFixture) expectStatus(t *testing.T, token string, want int) {
	t.Helper()
	rec := f.do(t, token, http.MethodGet, "/v1/me", "")
	if rec.Code != want {
		t.Fatalf("GET /v1/me = %d %s, want %d", rec.Code, rec.Body, want)
	}
	if want == http.StatusUnauthorized && decodeProblem(t, rec).Code != apibase.Unauthorized {
		t.Fatalf("GET /v1/me problem = %s, want unauthorized", rec.Body)
	}
}

func TestRevokeSessions_aTokenCopiedBeforeLogoutIsRefusedAfterIt(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"active", "suspended", "banned"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			f := newRevokeFixture(t, status)
			copied := f.token("session-phone", f.clock.Now())
			otherDevice := f.token("session-tablet", f.clock.Now())
			f.expectStatus(t, copied, http.StatusOK)
			if rec := f.revoke(t, copied, "this_session"); rec.Code != http.StatusNoContent {
				t.Fatalf("revoke this_session = %d %s, want 204", rec.Code, rec.Body)
			}
			f.expectStatus(t, copied, http.StatusUnauthorized)
			f.expectStatus(t, otherDevice, http.StatusOK)
			if rec := f.revoke(t, otherDevice, "this_session"); rec.Code != http.StatusNoContent {
				t.Fatalf("revoke the other device = %d %s, want 204", rec.Code, rec.Body)
			}
			f.expectStatus(t, otherDevice, http.StatusUnauthorized)
		})
	}
}

func TestRevokeSessions_signingOutEverywhereRefusesEveryEarlierTokenButNotALaterLogin(t *testing.T) {
	t.Parallel()
	f := newRevokeFixture(t, "active")
	phone := f.token("session-phone", f.clock.Now())
	tablet := f.token("session-tablet", f.clock.Now())
	f.clock.Advance(30 * time.Second)
	if rec := f.revoke(t, phone, "all_sessions"); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke all_sessions = %d %s, want 204", rec.Code, rec.Body)
	}
	f.expectStatus(t, phone, http.StatusUnauthorized)
	f.expectStatus(t, tablet, http.StatusUnauthorized)
	sameSecond := f.token("session-same-second", f.clock.Now())
	f.expectStatus(t, sameSecond, http.StatusUnauthorized)
	f.clock.Advance(2 * time.Second)
	f.expectStatus(t, f.token("session-new-login", f.clock.Now()), http.StatusOK)
}

func TestOpenSession_refusesATokenWhoseSessionWasRevoked(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.privy.Seed(alicePhone())
	me, err := f.open(t, alice)
	if err != nil {
		t.Fatalf("first open = %v", err)
	}
	revoke := app.NewRevokeSessionsHandler(app.RevokeSessionsDeps{UoW: db.New(f.pool, f.ids, f.clock), Clock: f.clock})
	err = revoke.Handle(t.Context(), app.RevokeSessions{
		UserID: me.ID, SessionID: "session-" + string(alice), Scope: app.RevokeThisSession,
	})
	if err != nil {
		t.Fatalf("revoke = %v", err)
	}
	if _, err := f.open(t, alice); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("open session with a revoked session = %v, want unauthorized", err)
	}
}

func TestRevokeSessions_refusesAScopeItDoesNotKnow(t *testing.T) {
	t.Parallel()
	f := newRevokeFixture(t, "active")
	token := f.token("session-phone", f.clock.Now())
	if rec := f.revoke(t, token, "everything"); rec.Code != http.StatusBadRequest {
		t.Fatalf("revoke with an unknown scope = %d %s, want 400", rec.Code, rec.Body)
	}
	f.expectStatus(t, token, http.StatusOK)
}

func TestRevokeSessions_aDevTokenHasNoSessionToEnd(t *testing.T) {
	t.Parallel()
	f := newRevokeFixture(t, "active")
	dev := f.devToken(t, f.user.ID.String())
	if rec := f.revoke(t, dev, "this_session"); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke this_session with a dev token = %d %s, want 204", rec.Code, rec.Body)
	}
	f.expectStatus(t, dev, http.StatusOK)
	var rows int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM revoked_sessions`).
		Scan(&rows); err != nil ||
		rows != 0 {
		t.Fatalf("revoked_sessions rows = %d, %v, want 0", rows, err)
	}
}

func TestRevokeSessions_endingASessionTwiceChangesNothingTheSecondTime(t *testing.T) {
	t.Parallel()
	f := newRevokeFixture(t, "active")
	token := f.token("session-phone", f.clock.Now())
	other := f.token("session-tablet", f.clock.Now())
	if rec := f.revoke(t, token, "this_session"); rec.Code != http.StatusNoContent {
		t.Fatalf("first revoke = %d %s", rec.Code, rec.Body)
	}
	for range 2 {
		if rec := f.revoke(
			t,
			other,
			"all_sessions",
		); rec.Code != http.StatusNoContent &&
			rec.Code != http.StatusUnauthorized {
			t.Fatalf("revoke all = %d %s", rec.Code, rec.Body)
		}
	}
	var sessions int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM revoked_sessions WHERE user_id = $1`,
		f.user.ID.UUID()).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatalf("revoked_sessions rows = %d, %v, want 1", sessions, err)
	}
}

func TestRevokeSessions_refusesWhatItCannotRevoke(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	handler := app.NewRevokeSessionsHandler(
		app.RevokeSessionsDeps{UoW: db.New(f.pool, testkit.NewIDs(9), f.clock), Clock: f.clock},
	)
	user := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	if err := handler.Handle(
		t.Context(),
		app.RevokeSessions{UserID: user.ID, Scope: "everything"},
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidInput {
		t.Fatalf("Handle with an unknown scope = %v, want invalid_input", err)
	}
	h := adapters.HTTP{Revoke: handler}
	if _, err := h.RevokeSessions(t.Context(), apiRevoke("this_session")); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("RevokeSessions without an actor = %v, want unauthorized", err)
	}
	if _, err := h.RevokeSessions(
		asUser(t, user.ID),
		apiRevoke("everything"),
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidInput {
		t.Fatalf("RevokeSessions with an unknown scope = %v, want invalid_input", err)
	}
	f.pool.Close()
	if err := handler.Handle(t.Context(), app.RevokeSessions{
		UserID: user.ID, SessionID: "session-phone", Scope: app.RevokeThisSession,
	}); err == nil {
		t.Fatal("Handle on a closed pool = nil, want the database error")
	}
}

func TestOpenSession_aDatabaseFailureCheckingTheSessionIsUnavailableNotUnauthorized(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.privy.Seed(alicePhone())
	closed, err := pgxpool.NewWithConfig(t.Context(), f.pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	handler := app.NewOpenSessionHandler(app.OpenSessionDeps{
		UoW: db.New(f.pool, f.ids, f.clock), Reads: closed, Users: adapters.Users{}, Links: adapters.Users{},
		Privy: f.privy, Wallets: f.rule, IDs: f.ids, Clock: f.clock, Hints: f.hints,
	})
	if _, err := handler.Handle(
		t.Context(),
		app.OpenSession{Token: string(alice)},
	); errs.CodeOf(
		err,
	) != errs.CodeDBUnavailable {
		t.Fatalf("open session with failing reads = %v, want db_unavailable", err)
	}
}

func apiRevoke(scope string) identityapi.RevokeSessionsRequestObject {
	return identityapi.RevokeSessionsRequestObject{
		Body: &identityapi.RevokeSessionsJSONRequestBody{Scope: identityapi.RevokeSessionsRequestScope(scope)},
	}
}
