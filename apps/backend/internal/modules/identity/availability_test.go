package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/ratelimit"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type failingHandleUsers struct{ err error }

type handleAttempt struct {
	user ids.UserID
	key  string
}

func (f failingHandleUsers) SetHandle(context.Context, sqlc.DBTX, ids.UserID, string, time.Time) error {
	return f.err
}

func (f httpFixture) setClaim(t *testing.T, id ids.UserID, handle, xName string, changed *time.Time) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(), `UPDATE users SET handle = NULLIF($2::text, ''),
		x_username = NULLIF($3::text, ''), handle_changed_at = $4 WHERE id = $1`,
		id.UUID(), handle, xName, changed)
	if err != nil {
		t.Fatal(err)
	}
}

func (f httpFixture) availability(t *testing.T, user ids.UserID, raw string) *httptest.ResponseRecorder {
	t.Helper()
	return f.askAvailability(t, f.handler, user, raw)
}

func (f httpFixture) setHandle(t *testing.T, user ids.UserID, handle string) *httptest.ResponseRecorder {
	t.Helper()
	return f.setHandleWithKey(t.Context(), t, user, handle, "set-"+handle)
}

func (f httpFixture) setHandleWithKey(
	ctx context.Context, t *testing.T, user ids.UserID, handle, key string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(
		ctx, http.MethodPut, "/v1/me/handle", strings.NewReader(`{"handle":"`+handle+`"}`),
	)
	req.Header.Set("Authorization", "Bearer "+f.verifier.Mint(user.String(), f.now.Add(time.Hour)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func TestSetHandle_concurrentClaimsProduceOneWinner(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	first := f.seed(t, portSeed{wallet: true})
	second := f.seed(t, portSeed{wallet: true})
	attempts := []handleAttempt{{first.ID, "race-first"}, {second.ID, "race-second"}}
	results, err := concurrency.FanOut(t.Context(), len(attempts), attempts,
		func(ctx context.Context, attempt handleAttempt) (*httptest.ResponseRecorder, error) {
			return f.setHandleWithKey(ctx, t, attempt.user, "race_handle", attempt.key), nil
		})
	if err != nil {
		t.Fatal(err)
	}
	var ok, taken int
	for _, rec := range results {
		switch rec.Code {
		case http.StatusOK:
			ok++
		case http.StatusUnprocessableEntity:
			if got := decodeProblem(t, rec); got.Code != api.HandleTaken {
				t.Fatalf("race problem = %s, want %s", got.Code, api.HandleTaken)
			}
			taken++
		default:
			t.Fatalf("race = %d %s", rec.Code, rec.Body)
		}
	}
	if ok != 1 || taken != 1 {
		t.Fatalf("race winners = %d success, %d taken; want one each", ok, taken)
	}
}

func TestLoadLockedHandleClaimFacts_mapsMissingAndDatabaseFailures(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	missing := f.newID(t)
	_, err := app.LoadLockedHandleClaimFacts(t.Context(), f.pool, missing, "missing_one")
	if errs.CodeOf(err) != errs.CodeUserNotFound || errors.Unwrap(err) != nil {
		t.Fatalf("missing locked facts = %v, want unwrapped user_not_found", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = app.LoadLockedHandleClaimFacts(ctx, f.pool, missing, "missing_one")
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("cancelled locked facts = %v, want internal", err)
	}
}

func TestSetHandle_updatesTheProfileAndRejectsTakenNames(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	first := f.seed(t, portSeed{wallet: true})
	second := f.seed(t, portSeed{wallet: true})
	if got := f.setHandle(t, first.ID, "kai_one"); got.Code != http.StatusOK {
		t.Fatalf("set = %d %s", got.Code, got.Body)
	}
	if got := f.setHandle(t, first.ID, "kai_one"); got.Code != http.StatusOK {
		t.Fatalf("same = %d %s", got.Code, got.Body)
	}
	if got := f.setHandle(t, second.ID, "admin"); decodeProblem(t, got).Code != api.HandleReserved {
		t.Fatalf("reserved = %d %s", got.Code, got.Body)
	}
	if got := f.setHandle(t, second.ID, "kai_one"); decodeProblem(t, got).Code != api.HandleTaken {
		t.Fatalf("taken = %d %s", got.Code, got.Body)
	}
	ready := f.now.Add(-domain.HandleChangeInterval)
	f.setClaim(t, first.ID, "kai_one", "", &ready)
	if got := f.setHandle(t, first.ID, "kai_two"); got.Code != http.StatusOK {
		t.Fatalf("rename = %d %s", got.Code, got.Body)
	}
	f.sameAvailability(t, second.ID, "kai_one", "kai_one", true, "")
}

func TestSetHandle_returnsWriteAndEventFailures(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{wallet: true})
	write := app.NewSetHandle(app.SetHandleDeps{
		UoW: db.New(f.pool, f.ids, testkit.NewClock(f.now)), Reads: f.pool,
		Users: failingHandleUsers{errs.New(errs.CodeInternal, "test.write")}, ClaimFacts: app.LoadHandleClaimFacts,
	})
	if _, err := write.Handle(t.Context(), u.ID, "write_one", f.now); err == nil {
		t.Fatal("write error = nil")
	}
	event := app.NewSetHandle(app.SetHandleDeps{
		UoW: db.New(f.pool, f.ids, testkit.NewClock(f.now)), Reads: f.pool, Users: adapters.Users{},
		ClaimFacts: app.LoadHandleClaimFacts,
	})
	const rejectEvent = `CREATE OR REPLACE FUNCTION reject_profile_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'event'; END $$; CREATE TRIGGER reject_profile_event BEFORE INSERT ON events FOR EACH ROW EXECUTE FUNCTION reject_profile_event()`
	if _, err := f.pool.Exec(t.Context(), rejectEvent); err != nil {
		t.Fatal(err)
	}
	if _, err := event.Handle(t.Context(), u.ID, "event_one", f.now); err == nil {
		t.Fatal("event error = nil")
	}
}

func TestSetHandle_coversInvalidNoopAndAdapterDuplicate(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	first := f.seed(t, portSeed{handle: "same_one", wallet: true})
	second := f.seed(t, portSeed{wallet: true})
	if got := f.setHandle(t, first.ID, "same_one"); got.Code != http.StatusOK {
		t.Fatal(got.Code)
	}
	if got := f.setHandle(t, second.ID, "ab"); decodeProblem(t, got).Code != api.HandleInvalid {
		t.Fatal(got.Code)
	}
	err := adapters.Users{}.SetHandle(t.Context(), f.pool, second.ID, "same_one", f.now)
	if errs.CodeOf(err) != errs.CodeHandleTaken {
		t.Fatal(err)
	}
}

func TestSetHandle_returnsClaimReadFailure(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{wallet: true})
	h := app.NewSetHandle(app.SetHandleDeps{
		UoW: db.New(f.pool, f.ids, testkit.NewClock(f.now)), Reads: f.pool, Users: adapters.Users{},
		ClaimFacts: func(context.Context, sqlc.DBTX, ids.UserID, string) (sqlc.HandleClaimFactsRow, error) {
			return sqlc.HandleClaimFactsRow{}, errs.New(errs.CodeInternal, "test.read")
		},
	})
	if _, err := h.Handle(t.Context(), u.ID, "read_one", f.now); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatal(err)
	}
}

func (f httpFixture) askAvailability(
	t *testing.T, h http.Handler, user ids.UserID, raw string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/v1/handles/"+url.PathEscape(raw)+"/availability", nil)
	if user != (ids.UserID{}) {
		req.Header.Set("Authorization", "Bearer "+f.verifier.Mint(user.String(), f.now.Add(time.Hour)))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func (f httpFixture) sameAvailability(
	t *testing.T, user ids.UserID, raw, handle string, available bool, reason string,
) {
	t.Helper()
	got, err := app.HandleAvailability(t.Context(), f.pool, user, raw, f.now)
	want := app.Availability{Handle: handle, Available: available, Reason: reason}
	if err != nil || got != want {
		t.Fatalf("HandleAvailability(%q) = %+v %v, want %+v", raw, got, err, want)
	}
	wantAvailability(t, f.availability(t, user, raw), handle, available, reason)
}

func wantAvailability(
	t *testing.T, rec *httptest.ResponseRecorder, handle string, available bool, reason string,
) {
	t.Helper()
	var got api.HandleAvailability
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET availability = %d %s, %v", rec.Code, rec.Body, err)
	}
	if got.Handle != handle || got.Available != available {
		t.Fatalf("GET availability = %+v, want handle %s available %v", got, handle, available)
	}
	switch {
	case reason == "" && got.Reason != nil:
		t.Fatalf("reason = %s, want none", *got.Reason)
	case reason != "" && (got.Reason == nil || string(*got.Reason) != reason):
		t.Fatalf("reason = %v, want %s", got.Reason, reason)
	}
}

func TestHandleAvailability_reportsTheReason(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	caller := f.seed(t, portSeed{wallet: true})
	f.seed(t, portSeed{handle: "heldone", wallet: true})
	f.seed(t, portSeed{handle: "taken_one", status: "deleted"})
	named := f.seed(t, portSeed{wallet: true})
	f.setClaim(t, named.ID, "", "Kaicenat", nil)
	changed := f.now
	window := f.now.Add(-domain.HandleChangeInterval)
	f.setClaim(t, caller.ID, "", "Monaco", nil)
	own := f.seed(t, portSeed{wallet: true})
	f.setClaim(t, own.ID, "admin", "", &changed)
	cooled := f.seed(t, portSeed{handle: "mine", wallet: true})
	f.setClaim(t, cooled.ID, "mine", "", &window)
	soon := f.seed(t, portSeed{handle: "mine_two", wallet: true})
	f.setClaim(t, soon.ID, "mine_two", "", &changed)
	unset := f.seed(t, portSeed{handle: "hasname", wallet: true})
	mover := f.seed(t, portSeed{handle: "oldname", wallet: true})
	fresh := f.seed(t, portSeed{wallet: true})
	long := strings.Repeat("z", 21)
	kelvin := "\u212Aai"

	cases := []struct {
		name, raw, handle, reason string
		available                 bool
		user                      ids.UserID
	}{
		{"free", "Kai_1", "kai_1", "", true, fresh.ID},
		{"folded", kelvin, "kai", "", true, fresh.ID},
		{"short", "Ab", "ab", "invalid", false, fresh.ID},
		{"punct", "Kai.Cenat", "kai.cenat", "invalid", false, fresh.ID},
		{"long", long, long, "invalid", false, fresh.ID},
		{"reserved", "admin", "admin", "reserved", false, fresh.ID},
		{"leet", "sh1t", "sh1t", "reserved", false, fresh.ID},
		{"code", "23456789", "23456789", "reserved", false, fresh.ID},
		{"held", "heldone", "heldone", "taken", false, fresh.ID},
		{"deleted", "taken_one", "taken_one", "taken", false, fresh.ID},
		{"other x", "kaicenat", "kaicenat", "taken", false, fresh.ID},
		{"own x skips the reserved list", "monaco", "monaco", "", true, caller.ID},
		{"own current handle", "admin", "admin", "", true, own.ID},
		{"own current handle inside the window", "mine_two", "mine_two", "", true, soon.ID},
		{"too soon", "next_one", "next_one", "too_soon", false, soon.ID},
		{"ready after the window", "next_one", "next_one", "", true, cooled.ID},
		{"a null change time does not block", "next_one", "next_one", "", true, unset.ID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f.sameAvailability(t, tc.user, tc.raw, tc.handle, tc.available, tc.reason)
		})
	}
	f.setClaim(t, mover.ID, "newname", "", &changed)
	f.sameAvailability(t, fresh.ID, "oldname", "oldname", true, "")
	f.sameAvailability(t, mover.ID, "newname", "newname", true, "")
}

func TestGetHandleAvailability_refusesWhoIsNotASignedInUser(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	missing := f.availability(t, ids.UserID{}, "kai_1")
	if got := decodeProblem(t, missing); missing.Code != http.StatusUnauthorized || got.Code != api.Unauthorized {
		t.Fatalf("no token = %d %s, want 401 unauthorized", missing.Code, missing.Body)
	}
	unknown := f.availability(t, f.newID(t), "kai_1")
	if got := decodeProblem(t, unknown); unknown.Code != http.StatusNotFound || got.Code != api.UserNotFound {
		t.Fatalf("unknown user = %d %s, want 404 user_not_found", unknown.Code, unknown.Body)
	}
}

func TestHTTP_availabilityRefusesCallersThatAreNotAUser(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{}
	req := api.GetHandleAvailabilityRequestObject{Handle: "kai_1"}
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
		if _, err := h.GetHandleAvailability(ctx, req); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: GetHandleAvailability err = %v, want %s", name, err, tc.want)
		}
		if _, err := h.PutMeHandle(ctx, api.PutMeHandleRequestObject{}); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: PutMeHandle err = %v, want %s", name, err, tc.want)
		}
	}
}

func TestHandleAvailability_aCancelledReadIsInternal(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	u := f.seed(t, portSeed{wallet: true})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := app.HandleAvailability(ctx, f.pool, u.ID, "kai_1", f.now)
	if err == nil || errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("HandleAvailability on a cancelled query = %v, want internal", err)
	}
}

func TestGetHandleAvailability_the31stCallInAMinuteIsRateLimited(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	user := f.seed(t, portSeed{wallet: true})
	other := f.seed(t, portSeed{wallet: true})
	h := f.limitedHandler(t)
	call := func(id ids.UserID) *httptest.ResponseRecorder {
		t.Helper()
		return f.askAvailability(t, h, id, "kai_1")
	}
	for n := 1; n <= 30; n++ {
		if rec := call(user.ID); rec.Code != http.StatusOK {
			t.Fatalf("call %d = %d %s, want 200", n, rec.Code, rec.Body)
		}
	}
	rec := call(user.ID)
	retry := rec.Header().Get("Retry-After")
	seconds, convErr := strconv.Atoi(retry)
	if got := decodeProblem(t, rec); rec.Code != http.StatusTooManyRequests || got.Code != api.RateLimited ||
		convErr != nil || seconds < 1 {
		t.Fatalf("call 31 = %d %s Retry-After %q, want 429 rate_limited with Retry-After", rec.Code, rec.Body, retry)
	}
	if rec := call(other.ID); rec.Code != http.StatusOK {
		t.Fatalf("another caller = %d %s, want 200", rec.Code, rec.Body)
	}
}

func (f httpFixture) limitedHandler(t *testing.T) http.Handler {
	t.Helper()
	policies, err := ratelimit.Load(openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	clk := testkit.NewClock(f.now)
	limiter, err := ratelimit.New(f.pool, clk, noop.NewMeterProvider())
	if err != nil {
		t.Fatal(err)
	}
	var routes httpx.Routes
	identity.New(module.Deps{
		Pool: f.pool, UoW: db.New(f.pool, f.ids, clk), IDs: f.ids, Clock: clk,
	}, identity.WithPrivy(f.privy, f.wallets)).Routes(&routes)
	h, err := httpx.Handler(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer:       tracenoop.NewTracerProvider(),
		Clock:        clk,
		IDs:          f.ids,
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(f.pool, clk),
		Verifier:     f.verifier,
		RateLimit:    ratelimit.Middleware(limiter, policies, httpx.ActorKey, false),
	}, routes, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return testkit.HTTP(t, h)
}
