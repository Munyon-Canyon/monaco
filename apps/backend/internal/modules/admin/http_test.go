package admin_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	adminapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type verifier func(context.Context, string) (auth.Actor, error)

func (v verifier) Verify(ctx context.Context, raw string) (auth.Actor, error) { return v(ctx, raw) }

func adminHandler(t *testing.T) http.Handler {
	t.Helper()
	return adminHandlerOn(t, testkit.DB(t))
}

func adminHandlerOn(t *testing.T, pool *pgxpool.Pool) http.Handler {
	t.Helper()
	clk := testkit.NewClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	v := verifier(func(_ context.Context, raw string) (auth.Actor, error) {
		switch raw {
		case "operator":
			return auth.Actor{Kind: auth.ActorAdmin, ID: "019cc330-1111-7000-8000-000000000001", Role: "operator"}, nil
		case "viewer":
			return auth.Actor{Kind: auth.ActorAdmin, ID: "019cc330-1111-7000-8000-000000000002", Role: "viewer"}, nil
		}
		return auth.Actor{}, errs.New(errs.CodeAdminForbidden, "test")
	})
	h, err := httpx.Handler(httpx.Deps{
		Logger:        observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer:        noop.NewTracerProvider(),
		Clock:         clk,
		IDs:           testkit.NewIDs(1),
		MaxBodyBytes:  1 << 20,
		Idempotency:   db.NewIdempotencyStore(pool, clk),
		Verifier:      v,
		AdminVerifier: v,
	}, admin.New(module.Deps{Pool: pool}).Mount, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return testkit.HTTP(t, h)
}

func adminRequest(t *testing.T, h http.Handler, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAdmin_UserToken_Forbidden(t *testing.T) {
	t.Parallel()
	if got := adminRequest(t, adminHandler(t), "/v1/admin/me", "user").Code; got != http.StatusForbidden {
		t.Fatalf("status = %d", got)
	}
}

func TestAdmin_ViewerOnOperatorRoute_Forbidden(t *testing.T) {
	t.Parallel()
	if got := adminRequest(t, adminHandler(t), "/v1/admin/admins", "viewer").Code; got != http.StatusForbidden {
		t.Fatalf("status = %d", got)
	}
}

func TestAdmin_OperatorOnViewerRoute_Ok(t *testing.T) {
	t.Parallel()
	if got := adminRequest(t, adminHandler(t), "/v1/admin/me", "operator").Code; got != http.StatusOK {
		t.Fatalf("status = %d", got)
	}
}

func TestAdmin_OperatorListsAdmins_Ok(t *testing.T) {
	t.Parallel()
	h := adminHandler(t)
	if got := adminRequest(t, h, "/v1/admin/admins", "operator").Code; got != http.StatusOK {
		t.Fatalf("status = %d", got)
	}
}

func TestAdminAdapter_ListErrors(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	h := adapters.HTTP{Pool: pool}
	req := adminapi.GetAdminsRequestObject{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := h.GetAdmins(ctx, req); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("query error = %v", err)
	}
	const insertViewer = `INSERT INTO admins (user_id, role, granted_at) VALUES ('019cc330-1111-7000-8000-000000000003', 'viewer', now())`
	if _, err := pool.Exec(t.Context(), insertViewer); err != nil {
		t.Fatal(err)
	}
	if _, err := h.GetAdmins(t.Context(), req); err != nil {
		t.Fatalf("list error = %v", err)
	}
	if _, err := pool.Exec(t.Context(), `ALTER TABLE admins ALTER COLUMN role DROP NOT NULL`); err != nil {
		t.Fatal(err)
	}
	const insertNullRole = `INSERT INTO admins (user_id, role, granted_at) VALUES ('019cc330-1111-7000-8000-000000000004', NULL, now())`
	if _, err := pool.Exec(t.Context(), insertNullRole); err != nil {
		t.Fatal(err)
	}
	if _, err := h.GetAdmins(t.Context(), req); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("scan error = %v", err)
	}
}

func TestAdminAdapter_RejectsMissingAndInvalidActors(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{Pool: testkit.DB(t)}
	req := adminapi.GetAdminMeRequestObject{}
	if _, err := h.GetAdminMe(t.Context(), req); errs.CodeOf(err) != errs.CodeAdminForbidden {
		t.Fatalf("missing actor error = %v", err)
	}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorAdmin, ID: "invalid", Role: "viewer"})
	if _, err := h.GetAdminMe(ctx, req); errs.CodeOf(err) != errs.CodeAdminForbidden {
		t.Fatalf("invalid actor error = %v", err)
	}
}
