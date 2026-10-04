package system_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/system"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/systemapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type server struct {
	fixture
	handler  http.Handler
	verifier *auth.DevVerifier
	clock    *testkit.Clock
}

func newServer(t *testing.T) server {
	t.Helper()
	f := newFixture(t)
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	verifier, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "test-only"}}, clk)
	if err != nil {
		t.Fatal(err)
	}
	mount := system.New(module.Deps{Pool: f.pool, UoW: f.uow, IDs: f.ids, Clock: clk}).Mount
	h, err := httpx.Handler(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer:       noop.NewTracerProvider(),
		Clock:        clk,
		IDs:          f.ids,
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(f.pool, clk),
		Verifier:     verifier,
	}, mount, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return server{fixture: f, handler: testkit.HTTP(t, h), verifier: verifier, clock: clk}
}

func (s server) do(t *testing.T, method, path, token, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func (s server) token(user string) string { return s.verifier.Mint(user, s.clock.Now().Add(time.Hour)) }

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
}

func pingOf(t *testing.T, rec *httptest.ResponseRecorder) api.Ping {
	t.Helper()
	var p api.Ping
	decode(t, rec, &p)
	return p
}

func problemOf(t *testing.T, rec *httptest.ResponseRecorder) apibase.ErrorCode {
	t.Helper()
	var p apibase.Problem
	decode(t, rec, &p)
	return p.Code
}

func TestPostSystemPing_recordsOnceAndReplaysTheSameResponseForTheSameKey(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	first := s.do(t, http.MethodPost, "/v1/system/pings", s.token(s.user.String()), "k1", `{"note":"hi"}`)
	replay := s.do(t, http.MethodPost, "/v1/system/pings", s.token(s.user.String()), "k1", `{"note":"hi"}`)
	if first.Code != http.StatusCreated || replay.Code != first.Code || replay.Body.String() != first.Body.String() {
		t.Fatalf(
			"POST twice = %d %s then %d %s, want one 201 replayed",
			first.Code,
			first.Body,
			replay.Code,
			replay.Body,
		)
	}
	ping := pingOf(t, first)
	if ping.Note != "hi" || ping.Echoed || s.eventCount(t) != 1 {
		t.Fatalf("ping = %+v with %d events, want an unechoed ping and one event", ping, s.eventCount(t))
	}
	got := s.do(t, http.MethodGet, "/v1/system/pings/"+ping.Id.String(), s.token(s.user.String()), "", "")
	if got.Code != http.StatusOK || pingOf(t, got) != ping {
		t.Fatalf("GET = %d %s, want 200 %+v", got.Code, got.Body, ping)
	}
	other := s.do(t, http.MethodGet, "/v1/system/pings/"+ping.Id.String(), s.token(userID(t, s.ids).String()), "", "")
	if other.Code != http.StatusNotFound || problemOf(t, other) != apibase.NotFound {
		t.Fatalf("GET as another user = %d %s, want 404 not_found", other.Code, other.Body)
	}
}

func TestPostSystemPing_refusesALongNoteAndAMissingToken(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	for name, tc := range map[string]struct {
		token, body string
		status      int
		code        apibase.ErrorCode
	}{
		"long note": {s.token(s.user.String()), `{"note":"` + strings.Repeat("a", 141) + `"}`, 400, apibase.InvalidInput},
		"no token":  {"", `{"note":"hi"}`, 401, apibase.Unauthorized},
	} {
		rec := s.do(t, http.MethodPost, "/v1/system/pings", tc.token, "k-"+name, tc.body)
		if rec.Code != tc.status || problemOf(t, rec) != tc.code {
			t.Errorf("%s: POST = %d %s, want %d %s", name, rec.Code, rec.Body, tc.status, tc.code)
		}
	}
	if n := s.eventCount(t); n != 0 {
		t.Fatalf("events rows = %d, want 0", n)
	}
}

func TestPostSystemPing_answersInternalWhenThePingCannotBeWritten(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	if _, err := s.pool.Exec(t.Context(), `ALTER TABLE system_pings RENAME TO system_pings_gone`); err != nil {
		t.Fatal(err)
	}
	rec := s.do(t, http.MethodPost, "/v1/system/pings", s.token(s.user.String()), "k1", `{"note":"hi"}`)
	if rec.Code != http.StatusInternalServerError || problemOf(t, rec) != apibase.Internal {
		t.Fatalf("POST = %d %s, want 500 internal", rec.Code, rec.Body)
	}
}

func TestHTTP_refusesCallersThatAreNotAUserAndNotesOver140(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{}
	body := &api.PingRequest{Note: strings.Repeat("a", 141)}
	for name, tc := range map[string]struct {
		actor *auth.Actor
		want  errs.Code
	}{
		"no actor":    {nil, errs.CodeUnauthorized},
		"agent":       {&auth.Actor{Kind: auth.ActorAgent, ID: "a1"}, errs.CodeForbidden},
		"bad user id": {&auth.Actor{Kind: auth.ActorUser, ID: "u1"}, errs.CodeUnauthorized},
		"long note":   {&auth.Actor{Kind: auth.ActorUser, ID: userID(t, testkit.NewIDs(2)).String()}, errs.CodeInvalidInput},
	} {
		ctx := t.Context()
		if tc.actor != nil {
			ctx = auth.WithActor(ctx, *tc.actor)
		}
		_, postErr := h.PostSystemPing(ctx, api.PostSystemPingRequestObject{Body: body})
		if errs.CodeOf(postErr) != tc.want {
			t.Errorf("%s: PostSystemPing err = %v, want %s", name, postErr, tc.want)
		}
		if tc.want == errs.CodeInvalidInput {
			continue
		}
		if _, err := h.GetSystemPing(ctx, api.GetSystemPingRequestObject{}); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: GetSystemPing err = %v, want %s", name, err, tc.want)
		}
	}
}
