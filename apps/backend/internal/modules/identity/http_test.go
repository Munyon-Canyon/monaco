package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/authn"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

type httpFixture struct {
	portFixture
	handler  http.Handler
	verifier *auth.DevVerifier
	privy    *privyfake.Users
	wallets  *privyfake.Wallets
}

func newHTTPFixture(t *testing.T) httpFixture {
	t.Helper()
	f := newPortFixture(t)
	clk := testkit.NewClock(f.now)
	verifier, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: devKey}}, clk)
	if err != nil {
		t.Fatal(err)
	}
	fakeUsers, fakeWallets := &privyfake.Users{}, &privyfake.Wallets{}
	var routes httpx.Routes
	identity.New(
		module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.ids, clk), IDs: f.ids, Clock: clk},
		identity.WithPrivy(fakeUsers, fakeWallets),
	).Routes(&routes)
	h, err := httpx.Handler(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer:       noop.NewTracerProvider(),
		Clock:        clk,
		IDs:          f.ids,
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(f.pool, clk),
		Verifier:     verifier,
	}, routes, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return httpFixture{
		portFixture: f, handler: testkit.HTTP(t, h), verifier: verifier, privy: fakeUsers, wallets: fakeWallets,
	}
}

func (f httpFixture) getMe(t *testing.T, user ids.UserID) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/me", nil)
	if user != (ids.UserID{}) {
		req.Header.Set("Authorization", "Bearer "+f.verifier.Mint(user.String(), f.now.Add(time.Hour)))
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func (f httpFixture) openSession(t *testing.T, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/auth/session", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func decodeMe(t *testing.T, rec *httptest.ResponseRecorder) api.Me {
	t.Helper()
	var me api.Me
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return me
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) api.Problem {
	t.Helper()
	var p api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return p
}

func TestGetMe_overHTTPServesTheAccount(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{
		handle: "kaicenat", name: "Kai Cenat", photo: "https://img.example/kai.png", authState: "AWAITING_SOCIALS",
		phoneHash: portHash("kai"), phoneVerified: true, wallet: true,
	})
	changed := f.now.Add(-48 * time.Hour)
	_, err := f.pool.Exec(t.Context(),
		`UPDATE users SET x_username = 'kai_on_x', handle_changed_at = $2 WHERE id = $1`, u.ID.UUID(), changed)
	if err != nil {
		t.Fatal(err)
	}
	rec := f.getMe(t, u.ID)
	handle, photo, x := "kaicenat", "https://img.example/kai.png", "kai_on_x"
	changeable := changed.Add(domain.HandleChangeInterval)
	want := api.Me{
		Id: u.ID.UUID(), Handle: &handle, DisplayName: "Kai Cenat", PhotoUrl: &photo,
		AuthState: api.AuthState(domain.AuthAwaitingSocials), AccountStatus: api.AccountStatus(domain.AccountActive),
		MemberWalletAddress: string(u.Address), PhoneLinked: true, XUsername: &x, HandleChangeableAt: &changeable,
		CreatedAt: f.created(),
	}
	got := decodeMe(t, rec)
	if rec.Code != http.StatusOK || !reflect.DeepEqual(normalized(got), normalized(want)) {
		t.Fatalf("GET /v1/me = %d %s, want %+v", rec.Code, rec.Body, want)
	}
}

func normalized(me api.Me) api.Me {
	me.CreatedAt = me.CreatedAt.UTC()
	if me.HandleChangeableAt != nil {
		at := me.HandleChangeableAt.UTC()
		me.HandleChangeableAt = &at
	}
	return me
}

func TestGetMe_overHTTPOmitsWhatIsUnset(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	bare := f.seed(t, portSeed{wallet: true})
	rec := f.getMe(t, bare.ID)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/me for a new user = %d %s, %v", rec.Code, rec.Body, err)
	}
	for _, unset := range []string{"handle", "photo_url", "x_username", "handle_changeable_at", "email", "phone"} {
		if _, ok := fields[unset]; ok {
			t.Fatalf("GET /v1/me for a new user carries %q: %s", unset, rec.Body)
		}
	}
	if string(fields["display_name"]) != `""` || string(fields["phone_linked"]) != "false" ||
		string(fields["auth_state"]) != `"CREATED"` {
		t.Fatalf("GET /v1/me for a new user = %s, want an empty display name, no phone and CREATED", rec.Body)
	}
}

func TestGetMe_overHTTPRefusesWhoIsNotASignedInUser(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	for name, tc := range map[string]struct {
		user   ids.UserID
		status int
		code   api.ErrorCode
	}{
		"no token":                 {ids.UserID{}, http.StatusUnauthorized, api.Unauthorized},
		"token without an account": {f.newID(t), http.StatusNotFound, api.UserNotFound},
	} {
		rec := f.getMe(t, tc.user)
		if got := decodeProblem(t, rec); rec.Code != tc.status || got.Code != tc.code {
			t.Errorf("%s: GET /v1/me = %d %s, want %d %s", name, rec.Code, rec.Body, tc.status, tc.code)
		}
	}
}

func TestHTTP_getMeRefusesCallersThatAreNotAUser(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{}
	for name, tc := range map[string]struct {
		actor *auth.Actor
		want  errs.Code
	}{
		"no actor":    {nil, errs.CodeUnauthorized},
		"agent":       {&auth.Actor{Kind: auth.ActorAgent, ID: "a1"}, errs.CodeForbidden},
		"admin":       {&auth.Actor{Kind: auth.ActorAdmin, ID: "a2"}, errs.CodeForbidden},
		"bad user id": {&auth.Actor{Kind: auth.ActorUser, ID: "u1"}, errs.CodeUnauthorized},
	} {
		ctx := t.Context()
		if tc.actor != nil {
			ctx = auth.WithActor(ctx, *tc.actor)
		}
		if _, err := h.GetMe(ctx, api.GetMeRequestObject{}); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: GetMe err = %v, want %s", name, err, tc.want)
		}
	}
}

func TestPostAuthSession_opensASessionWithNeitherAnIdempotencyKeyNorAnActor(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	f.privy.Seed(app.PrivyUser{ID: alice, PhoneE164: "+14155550100", Email: "alice@example.com"})
	rec := f.openSession(t, "Bearer "+string(alice))
	me := decodeMe(t, rec)
	if rec.Code != http.StatusOK || me.AuthState != api.AuthState(domain.AuthCreated) ||
		me.AccountStatus != api.AccountStatus(domain.AccountActive) || me.PhoneLinked || me.MemberWalletAddress == "" {
		t.Fatalf("POST /v1/auth/session = %d %s, want a CREATED active account with its wallet", rec.Code, rec.Body)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("alice@example.com")) ||
		bytes.Contains(rec.Body.Bytes(), []byte("4155550100")) {
		t.Fatalf("the response carries the email or the phone: %s", rec.Body)
	}
}

func TestPostAuthSession_aSecondCallAndGetMeReturnTheSameAccount(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	f.privy.Seed(app.PrivyUser{ID: alice, PhoneE164: "+14155550100"})
	first := f.openSession(t, "Bearer "+string(alice))
	again := f.openSession(t, "bearer "+string(alice))
	if first.Code != http.StatusOK || again.Code != http.StatusOK || again.Body.String() != first.Body.String() {
		t.Fatalf("POST twice = %d %s then %d %s, want the same account", first.Code, first.Body, again.Code, again.Body)
	}
	user, err := ids.ParseUserID(decodeMe(t, first).Id.String())
	if err != nil {
		t.Fatal(err)
	}
	if read := f.getMe(t, user); read.Code != http.StatusOK || read.Body.String() != first.Body.String() {
		t.Fatalf("GET /v1/me = %d %s, want the account the session returned", read.Code, read.Body)
	}
}

func TestPostAuthSession_refusesWhatItCannotSignIn(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		setup         func(f httpFixture)
		authorization string
		status        int
		code          api.ErrorCode
	}{
		"no header":     {func(httpFixture) {}, "", http.StatusUnauthorized, api.Unauthorized},
		"not a bearer":  {func(httpFixture) {}, "Basic YWxpY2U6eA==", http.StatusUnauthorized, api.Unauthorized},
		"empty bearer":  {func(httpFixture) {}, "Bearer ", http.StatusUnauthorized, api.Unauthorized},
		"unknown token": {func(httpFixture) {}, "Bearer nobody", http.StatusUnauthorized, api.Unauthorized},
		"no login method": {func(f httpFixture) {
			f.privy.Seed(app.PrivyUser{ID: alice, X: &domain.XAccount{UserID: "1", Username: "a"}})
		}, "Bearer " + string(alice), http.StatusForbidden, api.LoginMethodNotAllowed},
		"privy down": {func(f httpFixture) {
			f.privy.Seed(app.PrivyUser{ID: alice, PhoneE164: "+14155550100"})
			f.privy.Fail("User", errs.New(errs.CodePrivyUnavailable, "test"))
		}, "Bearer " + string(alice), http.StatusServiceUnavailable, api.PrivyUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newHTTPFixture(t)
			tc.setup(f)
			rec := f.openSession(t, tc.authorization)
			if got := decodeProblem(t, rec); rec.Code != tc.status || got.Code != tc.code {
				t.Fatalf("POST /v1/auth/session = %d %s, want %d %s", rec.Code, rec.Body, tc.status, tc.code)
			}
		})
	}
}

type openPing struct{}

func (openPing) PostSystemPing(context.Context, api.PostSystemPingRequestObject) (
	api.PostSystemPingResponseObject, error,
) {
	return api.PostSystemPing201JSONResponse{}, nil
}

func (openPing) GetSystemPing(context.Context, api.GetSystemPingRequestObject) (
	api.GetSystemPingResponseObject, error,
) {
	return api.GetSystemPing200JSONResponse{}, nil
}

func TestAccountStanding_changesTheNextResponseWithoutARestart(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	clk := testkit.NewClock(f.now)
	cfg := privyConfig()
	cfg.Env, cfg.Auth.DevTokenKey = config.EnvTest, devKey
	verifier, err := authn.New(cfg, clk, privyadapter.Users{Client: privyClient(t, cfg, clk)}, f.pool)
	if err != nil {
		t.Fatal(err)
	}
	var routes httpx.Routes
	identity.New(module.Deps{
		Config: cfg, Pool: f.pool, UoW: db.New(f.pool, f.ids, clk), IDs: f.ids, Clock: clk,
	}, identity.WithPrivy(&privyfake.Users{}, &privyfake.Wallets{})).Routes(&routes)
	routes.SystemRoutes = openPing{}
	handler, err := httpx.Handler(httpx.Deps{
		Logger: observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer: noop.NewTracerProvider(), Clock: clk, IDs: f.ids, MaxBodyBytes: 1 << 20,
		Idempotency: db.NewIdempotencyStore(f.pool, clk), Verifier: verifier,
	}, routes, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	user := testkit.SeedUser(t, f.pool, testkit.UserOpts{AccountStatus: "banned", WithWallet: true})
	token := fakes.PrivyAccessToken(privyAppID, user.PrivyUserID, clk.Now(), time.Hour)
	ping := "/v1/system/pings/" + testkit.NewIDs(9).NewV7().String()
	for _, step := range []standingStep{
		{method: http.MethodGet, path: "/v1/me", status: http.StatusOK},
		{method: http.MethodPost, path: "/v1/system/pings", body: `{"note":"out"}`, status: http.StatusForbidden, code: api.AccountBanned},
		{next: "suspended", method: http.MethodGet, path: ping, status: http.StatusOK},
		{method: http.MethodPost, path: "/v1/system/pings", body: `{"note":"out"}`, status: http.StatusForbidden, code: api.AccountSuspended},
		{next: "deleted", method: http.MethodGet, path: "/v1/me", status: http.StatusForbidden, code: api.AccountDeleted},
		{next: "active", method: http.MethodGet, path: ping, status: http.StatusOK},
	} {
		if step.next != "" {
			setAccountStatus(t, f, user.ID, step.next)
		}
		rec := callStanding(t, handler, token, step)
		got := api.ErrorCode("")
		if step.code != "" {
			got = decodeProblem(t, rec).Code
		}
		if rec.Code != step.status || got != step.code {
			t.Fatalf("%s %s after %q = %d %s, want %d %s",
				step.method, step.path, step.next, rec.Code, rec.Body, step.status, step.code)
		}
	}
}

type standingStep struct {
	next         string
	method, path string
	body         string
	status       int
	code         api.ErrorCode
}

func callStanding(t *testing.T, handler http.Handler, token string, step standingStep) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), step.method, step.path, strings.NewReader(step.body))
	req.Header.Set("Authorization", "Bearer "+token)
	if step.method == http.MethodPost {
		req.Header.Set("Idempotency-Key", "standing")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func setAccountStatus(t *testing.T, f portFixture, id ids.UserID, status string) {
	t.Helper()
	var deleted *time.Time
	if status == "deleted" {
		at := f.now
		deleted = &at
	}
	if _, err := f.pool.Exec(t.Context(),
		`UPDATE users SET account_status = $2, deleted_at = $3 WHERE id = $1`, id.UUID(), status, deleted); err != nil {
		t.Fatal(err)
	}
}
