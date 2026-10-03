package identity_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	tracenoop "go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

func TestDeleteMe_isUpstreamUnavailableUntilTheTreasuryPortIsWired(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{handle: "kai", name: "Kai", wallet: true})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+f.verifier.Mint(u.ID.String(), f.now.Add(time.Hour)))
	req.Header.Set("Idempotency-Key", "delete-1")
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || decodeProblem(t, rec).Code != api.UpstreamUnavailable {
		t.Fatalf("DELETE /v1/me = %d %s, want 503 upstream_unavailable", rec.Code, rec.Body)
	}
	if rec := f.getMe(t, u.ID); rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/me after a refused delete = %d %s, want 200", rec.Code, rec.Body)
	}
}

func TestDeleteMe_answers204OrTheCommandsError(t *testing.T) {
	t.Parallel()
	d := newDeleteFixture(t)
	u := d.seedWithPII(t, "active")
	h := adapters.HTTP{Delete: app.NewDeleteAccount(app.DeleteAccountDeps{
		UoW: db.New(d.pool, testkit.NewIDs(7), d.clock), Users: adapters.Users{}, Balances: fakes.NewBalances(),
		Stakes: d.treasury, Clock: d.clock, Hints: d.hints,
	})}
	if _, err := h.DeleteMe(t.Context(), api.DeleteMeRequestObject{}); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("DeleteMe without actor = %v, want Unauthorized", err)
	}
	d.stakeIn(t, u.ID, "01890a5d-ac96-774b-bcce-b302099a8003")
	if _, err := h.DeleteMe(asUser(t, u.ID), api.DeleteMeRequestObject{}); errs.CodeOf(
		err,
	) != errs.CodeAccountHasPositions {
		t.Fatalf("DeleteMe with a stake = %v, want AccountHasPositions", err)
	}
	other := d.seedWithPII(t, "banned")
	got, err := h.DeleteMe(asUser(t, other.ID), api.DeleteMeRequestObject{})
	if err != nil || got != (api.DeleteMe204Response{}) {
		t.Fatalf("DeleteMe = %#v, %v, want 204", got, err)
	}
}

func TestDeleteMe_theSamePrivyTokenIsAccountDeletedOnLaterRequests(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	user := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "del_token", WithWallet: true})
	var routes httpx.Routes
	identity.New(
		module.Deps{
			Pool:  f.pool,
			UoW:   db.New(f.pool, testkit.NewIDs(9), f.clock),
			IDs:   testkit.NewIDs(10),
			Clock: f.clock,
		},
		identity.WithPrivy(&privyfake.Users{}, &privyfake.Wallets{}),
		identity.WithHints(&recordedHints{}),
		identity.WithHoldings(fakes.NewTreasury(), fakes.NewBalances()),
	).Routes(&routes)
	h, err := httpx.Handler(httpx.Deps{
		Logger: observability.NewLogger(config.Config{Env: config.EnvTest}, &testkit.Logs{}),
		Tracer: tracenoop.NewTracerProvider(), Clock: f.clock, IDs: testkit.NewIDs(11), MaxBodyBytes: 1 << 20,
		Idempotency: db.NewIdempotencyStore(f.pool, f.clock), Verifier: f.verifier(t, config.EnvTest),
	}, routes, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	token := f.privyToken(user.PrivyUserID)
	for i, r := range []struct {
		method, path string
		status       int
	}{
		{http.MethodDelete, "/v1/me", http.StatusNoContent},
		{http.MethodGet, "/v1/me", http.StatusForbidden},
		{http.MethodDelete, "/v1/me", http.StatusForbidden},
	} {
		req := httptest.NewRequestWithContext(t.Context(), r.method, r.path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", "delete-"+strconv.Itoa(i))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != r.status ||
			r.status == http.StatusForbidden && decodeProblem(t, rec).Code != api.AccountDeleted {
			t.Fatalf("%s %s = %d %s, want %d", r.method, r.path, rec.Code, rec.Body, r.status)
		}
	}
}
