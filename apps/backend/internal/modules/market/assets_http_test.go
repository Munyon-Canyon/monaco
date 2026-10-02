package market_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestAssets_returnsAnEmptyPage(t *testing.T) {
	t.Parallel()
	handler, token := newAssetHandler(t)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/assets", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	var page api.AssetList
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Assets) != 0 || page.NextCursor != nil {
		t.Fatalf("page = %+v", page)
	}
	anon := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/assets", nil)
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, anon)
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d %s", denied.Code, denied.Body)
	}
}

func TestAssets_rejectsACallerThatIsNotAUser(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{}
	if _, err := h.GetAssets(t.Context(), api.GetAssetsRequestObject{}); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("no actor = %v", err)
	}
	agent := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorAgent, ID: "agent"})
	if _, err := h.GetAssets(agent, api.GetAssetsRequestObject{}); errs.CodeOf(err) != errs.CodeForbidden {
		t.Fatalf("agent = %v", err)
	}
	bad := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: "not-a-uuid"})
	if _, err := h.GetAssets(bad, api.GetAssetsRequestObject{}); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("bad user = %v", err)
	}
}

func newAssetHandler(t *testing.T) (http.Handler, string) {
	t.Helper()
	when := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	clk := testkit.NewClock(when)
	pool := testkit.DB(t)
	verifier, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "test-only"}}, clk)
	if err != nil {
		t.Fatal(err)
	}
	var routes httpx.Routes
	market.New(module.Deps{Pool: pool, Clock: clk}).Routes(&routes)
	handler, err := httpx.Handler(httpx.Deps{
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
	return testkit.HTTP(t, handler), verifier.Mint(testkit.NewIDs(2).NewV7().String(), when.Add(time.Hour))
}
