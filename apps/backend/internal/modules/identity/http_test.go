package identity_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
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
)

type httpFixture struct {
	portFixture
	handler  http.Handler
	verifier *auth.DevVerifier
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
	var routes httpx.Routes
	identity.New(module.Deps{Pool: f.pool, Clock: clk}).Routes(&routes)
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
	return httpFixture{portFixture: f, handler: testkit.HTTP(t, h), verifier: verifier}
}

func (f httpFixture) get(t *testing.T, path string, user ids.UserID) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	if user != (ids.UserID{}) {
		req.Header.Set("Authorization", "Bearer "+f.verifier.Mint(user.String(), f.now.Add(time.Hour)))
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
	rec := f.get(t, "/v1/me", u.ID)
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
	rec := f.get(t, "/v1/me", bare.ID)
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
		rec := f.get(t, "/v1/me", tc.user)
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
