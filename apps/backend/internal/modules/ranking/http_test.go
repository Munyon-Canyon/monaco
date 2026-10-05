package ranking_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type server struct {
	handler  http.Handler
	verifier *auth.DevVerifier
	clock    *testkit.Clock
	pool     *pgxpool.Pool
}

func newServer(t *testing.T) server {
	t.Helper()
	g := testkit.NewIDs(1)
	pool := testkit.DB(t)
	clk := testkit.NewClock(time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC))
	verifier, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "test-only"}}, clk)
	if err != nil {
		t.Fatal(err)
	}
	deps := module.Deps{Pool: pool, IDs: g, Clock: clk}
	rankingModule := ranking.New(deps)
	module.NewSet(rankingModule, cabal.New(deps))
	mount := rankingModule.Mount
	h, err := httpx.Handler(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer:       noop.NewTracerProvider(),
		Clock:        clk,
		IDs:          g,
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(pool, clk),
		Verifier:     verifier,
	}, mount, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return server{handler: testkit.HTTP(t, h), verifier: verifier, clock: clk, pool: pool}
}

func (s server) get(t *testing.T, path string, user ids.UserID) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	if user != (ids.UserID{}) {
		req.Header.Set("Authorization", "Bearer "+s.verifier.Mint(user.String(), s.clock.Now().Add(time.Hour)))
	}
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
}

func problemOf(t *testing.T, rec *httptest.ResponseRecorder) apibase.ErrorCode {
	t.Helper()
	var p apibase.Problem
	decode(t, rec, &p)
	return p.Code
}

func pageOf(t *testing.T, rec *httptest.ResponseRecorder) api.LeaderboardPage {
	t.Helper()
	var p api.LeaderboardPage
	decode(t, rec, &p)
	return p
}
