package trading_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/tradingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type retryServer struct {
	*retryEnv
	handler  http.Handler
	verifier *auth.DevVerifier
	clock    *testkit.Clock
}

func newRetryServer(t *testing.T) retryServer {
	t.Helper()
	e := newRetryEnv(t)
	clk := testkit.NewClock(e.now)
	cfg := config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "test-only"}}
	verifier, err := auth.NewDevVerifier(cfg, clk)
	if err != nil {
		t.Fatal(err)
	}
	m := trading.New(module.Deps{Config: cfg, Pool: e.pool, UoW: db.New(e.pool, e.ids, clk), IDs: e.ids, Clock: clk},
		trading.WithEnginePorts(trading.EnginePorts{Cabals: e.cabals, Proposals: e.proposals}))
	h, err := httpx.Handler(httpx.Deps{
		Logger: observability.NewLogger(cfg, io.Discard), Tracer: noop.NewTracerProvider(), Clock: clk, IDs: e.ids,
		MaxBodyBytes: 1 << 20, Idempotency: db.NewIdempotencyStore(e.pool, clk), Verifier: verifier,
	}, m.Mount, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return retryServer{retryEnv: e, handler: testkit.HTTP(t, h), verifier: verifier, clock: clk}
}

func (s retryServer) post(t *testing.T, swap, token, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/swaps/"+swap+"/retry", http.NoBody)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func (s retryServer) token() string {
	return s.verifier.Mint(s.member.String(), s.clock.Now().Add(time.Hour))
}

func TestRetryHTTP_acceptsAndReplaysTheSameKey(t *testing.T) {
	t.Parallel()
	s := newRetryServer(t)
	first := s.post(t, s.failed.ID.String(), s.token(), "r1")
	replay := s.post(t, s.failed.ID.String(), s.token(), "r1")
	want := `{"status":"retry_requested","swap_id":"` + s.failed.ID.String() + `"}`
	if first.Code != http.StatusAccepted || first.Body.String() != want+"\n" {
		t.Fatalf("first = %d %s, want 202 %s", first.Code, first.Body, want)
	}
	if replay.Code != first.Code || replay.Body.String() != first.Body.String() {
		t.Fatalf("replay = %d %s, want the first response", replay.Code, replay.Body)
	}
	if n := len(s.requests(t, s.failed.ID)); n != 1 {
		t.Fatalf("appended %d trade.retry_requested, want 1", n)
	}
}

func TestRetryHTTP_refusals(t *testing.T) {
	t.Parallel()
	s := newRetryServer(t)
	stranger := s.verifier.Mint(s.ids.NewV7().String(), s.clock.Now().Add(time.Hour))
	cases := map[string]struct {
		swap, token string
		want        int
	}{
		"no token":     {swap: s.failed.ID.String(), want: http.StatusUnauthorized},
		"unknown swap": {swap: s.ids.NewV7().String(), token: s.token(), want: http.StatusNotFound},
		"not a member": {swap: s.failed.ID.String(), token: stranger, want: http.StatusForbidden},
	}
	for name, tc := range cases {
		if rec := s.post(t, tc.swap, tc.token, "k-"+name); rec.Code != tc.want {
			t.Errorf("%s: status = %d %s, want %d", name, rec.Code, rec.Body, tc.want)
		}
	}
}

func TestRetryHTTP_callerMustBeAUser(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		actor auth.Actor
		want  errs.Code
	}{
		"system actor":   {actor: auth.Actor{Kind: auth.ActorSystem, ID: "trading.engine"}, want: errs.CodeForbidden},
		"malformed user": {actor: auth.Actor{Kind: auth.ActorUser, ID: "not-a-uuid"}, want: errs.CodeUnauthorized},
	}
	_, err := adapters.HTTP{}.PostSwapRetry(t.Context(), tradingapi.PostSwapRetryRequestObject{})
	if errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Errorf("no actor: err = %v, want unauthorized", err)
	}
	for name, tc := range cases {
		ctx := auth.WithActor(t.Context(), tc.actor)
		_, err := adapters.HTTP{}.PostSwapRetry(ctx, tradingapi.PostSwapRetryRequestObject{})
		if errs.CodeOf(err) != tc.want {
			t.Errorf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
}
