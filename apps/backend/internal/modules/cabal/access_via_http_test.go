package cabal_test

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
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f accessFixture) router(t *testing.T, codes app.CodeResolver) (http.Handler, *auth.DevVerifier) {
	t.Helper()
	verifier, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "test-only"}}, f.clock)
	if err != nil {
		t.Fatal(err)
	}
	routes := f.routes(nil)
	routes.Request = app.NewRequestAccessHandler(f.uow, f.ids, f.clock, codes)
	h, err := httpx.Handler(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer:       noop.NewTracerProvider(),
		Clock:        f.clock,
		IDs:          f.ids,
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(f.pool, f.clock),
		Verifier:     verifier,
	}, func(m apibase.Mount) { api.Mount(routes, m) }, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return testkit.HTTP(t, h), verifier
}

func (f accessFixture) call(
	t *testing.T, h http.Handler, v *auth.DevVerifier, user ids.UserID, method, path, key, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+v.Mint(user.String(), f.clock.Now().Add(time.Hour)))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPostCabalAccessRequest_acceptsNoBodyAnEmptyBodyAndACode(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithMembers(2))
	jordan := c.Members[1].ID
	f.makeAdmin(t, c.ID, jordan)
	f.exec(t, `UPDATE users SET display_name = 'Jordan' WHERE id = $1`, jordan.UUID())
	h, v := f.router(t, fakeCodes{owners: map[string]ids.UserID{"jordan": jordan}})
	path := "/v1/cabals/" + c.ID.String() + "/access-requests"

	for i, body := range []string{"", "{}", `{"via_code": "jordan"}`} {
		asker := f.user(t)
		rec := f.call(t, h, v, asker, http.MethodPost, path, "k"+string(rune('a'+i)), body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST with body %q = %d %s, want 201", body, rec.Code, rec.Body)
		}
	}

	rec := f.call(t, h, v, jordan, http.MethodGet, path, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s, want 200", rec.Code, rec.Body)
	}
	var listed []api.CabalAccessRequest
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	if len(listed) != 3 || listed[0].Via == nil || listed[0].Via.DisplayName != "Jordan" ||
		listed[1].Via != nil || listed[2].Via != nil {
		t.Fatalf("listed = %+v, want the newest via Jordan and the other two without", listed)
	}
}
