package social_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/ratelimit"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f contactFixture) contactRoutes() adapters.HTTP {
	deps := module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock}
	return social.HTTPOf(social.New(deps, social.WithUsers(f.readers)))
}

func TestMatchContacts_Validation(t *testing.T) {
	t.Parallel()
	f := newContactFixture(t)
	me := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "validator"})
	valid := contactHash("+14155550123")
	tooMany := make([]string, domain.MaxContactHashes+1)
	for i := range tooMany {
		tooMany[i] = valid
	}
	routes := f.contactRoutes()
	ctx := asUser(t.Context(), me.ID)
	tests := []struct {
		name string
		body *api.ContactMatchRequest
		want errs.Code
	}{
		{"empty", &api.ContactMatchRequest{Hashes: []string{}}, errs.CodeContactHashesInvalid},
		{"missing", nil, errs.CodeContactHashesInvalid},
		{
			"uppercase",
			&api.ContactMatchRequest{Hashes: []string{strings.ToUpper(valid)}},
			errs.CodeContactHashesInvalid,
		},
		{"short", &api.ContactMatchRequest{Hashes: []string{valid[:63]}}, errs.CodeContactHashesInvalid},
		{"too many", &api.ContactMatchRequest{Hashes: tooMany}, errs.CodeTooManyContactHashes},
	}
	for _, tt := range tests {
		_, err := routes.PostMeContactsMatch(ctx, api.PostMeContactsMatchRequestObject{Body: tt.body})
		if got := errs.CodeOf(err); err == nil || got != tt.want {
			t.Errorf("%s: err = %v (code %s), want %s", tt.name, err, got, tt.want)
		}
	}
}

func TestPostMeContactsMatch_returnsTheSamePageAsTheGet(t *testing.T) {
	t.Parallel()
	f := newContactFixture(t)
	me := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "poster"})
	named := seedPhoneUser(t, f.pool, "named_pal", "+14155554001", "active", true)
	bare := seedPhoneUser(t, f.pool, "bare_pal", "+14155554002", "active", true)
	if _, err := f.pool.Exec(t.Context(), `UPDATE users SET display_name = 'Named Pal', photo_url = $2 WHERE id = $1`,
		named.UUID(), "https://img.example/named.png"); err != nil {
		t.Fatal(err)
	}
	ctx := asUser(t.Context(), me.ID)
	routes := f.contactRoutes()
	hashes := []string{contactHash("+14155554001"), contactHash("+14155554002")}
	postContactPage(ctx, t, routes, hashes)
	first, rest := pageContactMatches(ctx, t, routes)
	assertNamedAndBare(t, first.Items, rest.Items, bare)
	assertCursorAndLimitErrors(ctx, t, routes)
}

func TestContactRoutes_refuseACallerThatIsNotASignedInUser(t *testing.T) {
	t.Parallel()
	f := newContactFixture(t)
	routes := f.contactRoutes()
	hash := contactHash("+14155550123")
	tests := []struct {
		name  string
		actor *auth.Actor
		want  errs.Code
	}{
		{"no actor", nil, errs.CodeUnauthorized},
		{"admin actor", &auth.Actor{Kind: auth.ActorAdmin, ID: f.alice.String()}, errs.CodeForbidden},
		{"malformed actor id", &auth.Actor{Kind: auth.ActorUser, ID: "not-a-uuid"}, errs.CodeUnauthorized},
	}
	for _, tt := range tests {
		ctx := t.Context()
		if tt.actor != nil {
			ctx = auth.WithActor(ctx, *tt.actor)
		}
		_, err := routes.PostMeContactsMatch(ctx, api.PostMeContactsMatchRequestObject{
			Body: &api.ContactMatchRequest{Hashes: []string{hash}},
		})
		if got := errs.CodeOf(err); err == nil || got != tt.want {
			t.Errorf("match, %s: err = %v (code %s), want %s", tt.name, err, got, tt.want)
		}
		_, err = routes.GetMeContactsMatches(ctx, api.GetMeContactsMatchesRequestObject{})
		if got := errs.CodeOf(err); err == nil || got != tt.want {
			t.Errorf("list, %s: err = %v (code %s), want %s", tt.name, err, got, tt.want)
		}
	}
}

func TestContactRoutes_aMissingTableIsInternal(t *testing.T) {
	t.Parallel()
	f := newContactFixture(t)
	me := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "dropper"})
	seedPhoneUser(t, f.pool, "drop_pal", "+14155555001", "active", true)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE contact_matches`); err != nil {
		t.Fatal(err)
	}
	ctx := asUser(t.Context(), me.ID)
	routes := f.contactRoutes()
	_, err := routes.PostMeContactsMatch(ctx, api.PostMeContactsMatchRequestObject{
		Body: &api.ContactMatchRequest{Hashes: []string{contactHash("+14155555001")}},
	})
	wantCode(t, err, errs.CodeInternal)
	_, err = routes.PostMeContactsMatch(ctx, api.PostMeContactsMatchRequestObject{
		Body: &api.ContactMatchRequest{Hashes: []string{contactHash("+19995550000")}},
	})
	wantCode(t, err, errs.CodeInternal)
	_, err = routes.GetMeContactsMatches(ctx, api.GetMeContactsMatchesRequestObject{})
	wantCode(t, err, errs.CodeInternal)
}

func TestMatchContacts_RateLimited(t *testing.T) {
	t.Parallel()
	f := newContactFixture(t)
	me := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "limited"})
	other := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "other_ip"})
	h, verifier, logs := f.limitedHandler(t)
	hash := contactHash("+19995550111")
	call := func(id ids.UserID, n int) *httptest.ResponseRecorder {
		t.Helper()
		return postMatch(t, h, verifier, f.now, id, n, hash)
	}
	for n := 1; n <= 10; n++ {
		if rec := call(me.ID, n); rec.Code != http.StatusOK {
			t.Fatalf("call %d = %d %s, want 200", n, rec.Code, rec.Body)
		}
	}
	rec := call(me.ID, 11)
	retry := rec.Header().Get("Retry-After")
	seconds, convErr := strconv.Atoi(retry)
	if got := decodeProblem(t, rec); rec.Code != http.StatusTooManyRequests || got != string(apibase.RateLimited) ||
		convErr != nil || seconds < 1 {
		t.Fatalf("call 11 = %d %s Retry-After %q, want 429 rate_limited", rec.Code, rec.Body, retry)
	}
	if rec := call(other.ID, 1); rec.Code != http.StatusOK {
		t.Fatalf("another caller = %d %s, want 200", rec.Code, rec.Body)
	}
	if strings.Contains(string(logs.Bytes()), hash) {
		t.Fatal("rate-limit log contains the contact hash")
	}
}

func TestMatchContacts_RedactsRequestHashes(t *testing.T) {
	t.Parallel()
	f := newContactFixture(t)
	me := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "redactor"})
	friend := seedPhoneUser(t, f.pool, "red_pal", "+14155556001", "active", true)
	h, verifier, logs := f.limitedHandler(t)
	hash := contactHash("+14155556001")
	rec := postMatch(t, h, verifier, f.now, me.ID, 1, hash)
	if rec.Code != http.StatusOK {
		t.Fatalf("match = %d %s", rec.Code, rec.Body)
	}
	logged := string(logs.Bytes())
	if strings.Contains(logged, hash) || !strings.Contains(logged, "social.contacts_matched") {
		t.Fatalf("log = %s, want the match count without the hash", logged)
	}
	if rows := f.matchRows(t, me.ID); len(rows) != 1 || rows[0] != friend.UUID() {
		t.Fatalf("rows = %v, want %s", rows, friend)
	}
}

func postContactPage(ctx context.Context, t *testing.T, routes adapters.HTTP, hashes []string) {
	t.Helper()
	res, err := routes.PostMeContactsMatch(ctx, api.PostMeContactsMatchRequestObject{
		Body: &api.ContactMatchRequest{Hashes: hashes},
	})
	if err != nil {
		t.Fatal(err)
	}
	posted, ok := res.(api.PostMeContactsMatch200JSONResponse)
	if !ok || len(posted.Items) != 2 || posted.NextCursor != nil {
		t.Fatalf("post = %#v, want two matches", res)
	}
}

func pageContactMatches(
	ctx context.Context, t *testing.T, routes adapters.HTTP,
) (first, rest api.GetMeContactsMatches200JSONResponse) {
	t.Helper()
	limit := 1
	paged, err := routes.GetMeContactsMatches(ctx, api.GetMeContactsMatchesRequestObject{
		Params: api.GetMeContactsMatchesParams{Limit: &limit},
	})
	if err != nil {
		t.Fatal(err)
	}
	var ok bool
	first, ok = paged.(api.GetMeContactsMatches200JSONResponse)
	if !ok || len(first.Items) != 1 || first.NextCursor == nil {
		t.Fatalf("get = %#v, want one item and a cursor", paged)
	}
	second, err := routes.GetMeContactsMatches(ctx, api.GetMeContactsMatchesRequestObject{
		Params: api.GetMeContactsMatchesParams{Cursor: first.NextCursor},
	})
	if err != nil {
		t.Fatal(err)
	}
	rest, ok = second.(api.GetMeContactsMatches200JSONResponse)
	if !ok || len(rest.Items) != 1 || rest.NextCursor != nil {
		t.Fatalf("second = %#v, want the other match", second)
	}
	return first, rest
}

func assertNamedAndBare(t *testing.T, first, rest []api.ContactMatch, bare ids.UserID) {
	t.Helper()
	var sawBare, sawNamed bool
	for _, item := range append(append([]api.ContactMatch{}, first...), rest...) {
		sawBare = sawBare || bareContact(item, bare)
		sawNamed = sawNamed || namedContact(item)
	}
	if !sawBare || !sawNamed {
		t.Fatalf("items = %+v %+v, want both people", first, rest)
	}
}

func bareContact(item api.ContactMatch, bare ids.UserID) bool {
	return item.Handle == "bare_pal" && item.PhotoUrl == nil && !item.FollowedByMe &&
		item.UserId.String() == bare.String()
}

func namedContact(item api.ContactMatch) bool {
	return item.Handle == "named_pal" && item.PhotoUrl != nil && *item.PhotoUrl == "https://img.example/named.png" &&
		item.DisplayName == "Named Pal"
}

func assertCursorAndLimitErrors(ctx context.Context, t *testing.T, routes adapters.HTTP) {
	t.Helper()
	blank := ""
	blankPage, err := routes.GetMeContactsMatches(ctx, api.GetMeContactsMatchesRequestObject{
		Params: api.GetMeContactsMatchesParams{Cursor: &blank},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := blankPage.(api.GetMeContactsMatches200JSONResponse); !ok || len(got.Items) != 2 {
		t.Fatalf("blank cursor = %#v, want both matches", blankPage)
	}
	bad := "not-a-cursor"
	_, err = routes.GetMeContactsMatches(ctx, api.GetMeContactsMatchesRequestObject{
		Params: api.GetMeContactsMatchesParams{Cursor: &bad},
	})
	wantCode(t, err, errs.CodeInvalidInput)
	zero := 0
	_, err = routes.GetMeContactsMatches(ctx, api.GetMeContactsMatchesRequestObject{
		Params: api.GetMeContactsMatchesParams{Limit: &zero},
	})
	wantCode(t, err, errs.CodeInvalidInput)
}

func postMatch(
	t *testing.T, h http.Handler, verifier *auth.DevVerifier, now time.Time, user ids.UserID, n int, hash string,
) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte(`{"hashes":["` + hash + `"]}`)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/me/contacts/match", bytes.NewReader(body))
	req.RemoteAddr = "192.0.2.8:443"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", fmt.Sprintf("%s-%d", user.String(), n))
	req.Header.Set("Authorization", "Bearer "+verifier.Mint(user.String(), now.Add(time.Hour)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Code
}

func (f contactFixture) limitedHandler(t *testing.T) (http.Handler, *auth.DevVerifier, *testkit.Logs) {
	t.Helper()
	logs := &testkit.Logs{}
	verifier, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "contact-match-dev-key"}}, f.clock)
	if err != nil {
		t.Fatal(err)
	}
	limiter, err := ratelimit.New(f.pool, f.clock, noop.NewMeterProvider())
	if err != nil {
		t.Fatal(err)
	}
	mount := social.New(module.Deps{
		Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock,
	}, social.WithUsers(f.readers)).Mount
	h, err := httpx.HandlerFor(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, logs),
		Tracer:       tracenoop.NewTracerProvider(),
		Clock:        f.clock,
		IDs:          f.gen,
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(f.pool, f.clock),
		Verifier:     verifier,
		RateLimit:    ratelimit.Middleware(limiter, socialPolicies(t), httpx.ActorKey, false),
	}, mount, socialContract(t))
	if err != nil {
		t.Fatal(err)
	}
	return testkit.HTTP(t, h), verifier, logs
}

func socialContract(t *testing.T) *httpx.Contract {
	t.Helper()
	c, err := socialContractOnce()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func socialPolicies(t *testing.T) ratelimit.Policies {
	t.Helper()
	p, err := socialPoliciesOnce()
	if err != nil {
		t.Fatal(err)
	}
	return p
}
