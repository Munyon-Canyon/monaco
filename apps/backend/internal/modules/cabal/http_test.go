package cabal_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type failCards struct{}

func (failCards) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]port.UserCard, error) {
	return nil, errs.New(errs.CodeInternal, "test.cards")
}

func (f createFixture) routes(users app.UserCards) adapters.HTTP {
	if users == nil {
		users = identity.New(module.Deps{Pool: f.pool}).Queries()
	}
	return adapters.HTTP{Create: f.handler(), DB: f.pool, Users: users}
}

func (f createFixture) router(t *testing.T) (http.Handler, *auth.DevVerifier) {
	t.Helper()
	verifier, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "test-only"}}, f.clock)
	if err != nil {
		t.Fatal(err)
	}
	h, err := httpx.Handler(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer:       noop.NewTracerProvider(),
		Clock:        f.clock,
		IDs:          f.ids,
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(f.pool, f.clock),
		Verifier:     verifier,
	}, httpx.Routes{CabalRoutes: f.routes(nil)}, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return testkit.HTTP(t, h), verifier
}

func cabalPost(
	key, name, join, voters, threshold string, expiry int32, slippage *int32,
) api.PostCabalRequestObject {
	return api.PostCabalRequestObject{
		Params: api.PostCabalParams{IdempotencyKey: key},
		Body: &api.CreateCabalRequest{
			Name: name, JoinMode: join, VoterMode: voters, Threshold: threshold,
			ProposalExpirySeconds: expiry, SlippageBps: slippage,
		},
	}
}

func asCabal(t *testing.T, res api.PostCabalResponseObject, err error) api.Cabal {
	t.Helper()
	posted, ok := res.(api.PostCabal201JSONResponse)
	if err != nil || !ok {
		t.Fatalf("PostCabal = %T, %v", res, err)
	}
	return api.Cabal(posted)
}

func TestPostCabal_returnsTheCabalTheCreatorJustOpened(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	setProfile(t, f, f.user.ID, "kai", "Kai Cenat", "https://cdn.example/kai.jpg")
	slippage := int32(50)
	res, err := f.routes(nil).PostCabal(
		f.actor(t.Context()),
		cabalPost("c1", "Friends pot", "request", "list", "unanimous", domain.ExpiryWeek, &slippage),
	)
	got := asCabal(t, res, err)
	assertPostedCabal(t, f, got)
	assertPostedCreator(t, f, got)
}

func assertPostedCabal(t *testing.T, f createFixture, got api.Cabal) {
	t.Helper()
	wallet, err := f.wallets.CreateAppWallet(t.Context(), app.TreasuryKey(f.user.ID, "c1"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Friends pot" || got.Status != "active" || got.PictureUrl != nil || got.MemberCount != 1 {
		t.Fatalf("cabal = %+v", got)
	}
	if got.Rules.JoinMode != "request" || got.Rules.VoterMode != "list" || got.Rules.Threshold != "unanimous" {
		t.Fatalf("rules = %+v", got.Rules)
	}
	if got.Rules.ProposalExpirySeconds != domain.ExpiryWeek || got.Rules.SlippageBps != 50 {
		t.Fatalf("rules = %+v", got.Rules)
	}
	assertPostedAccess(t, f, got, string(wallet.Address))
}

func assertPostedAccess(t *testing.T, f createFixture, got api.Cabal, address string) {
	t.Helper()
	if got.TreasuryAddress != address || f.wallets.Creates() != 1 || got.MyAccessRequest != nil {
		t.Fatalf("wallet %s creates %d access %v", got.TreasuryAddress, f.wallets.Creates(), got.MyAccessRequest)
	}
	if got.InviteCode == nil || *got.InviteCode == "" || got.Me == nil {
		t.Fatalf("invite %v me %v", got.InviteCode, got.Me)
	}
	if got.Me.Role != "creator" || !got.Me.CanVote {
		t.Fatalf("me = %+v", got.Me)
	}
}

func assertPostedCreator(t *testing.T, f createFixture, got api.Cabal) {
	t.Helper()
	if got.Creator.UserId != f.user.ID.UUID() || deref(got.Creator.Handle) != "kai" {
		t.Fatalf("creator = %+v", got.Creator)
	}
	if got.Creator.DisplayName != "Kai Cenat" || deref(got.Creator.PhotoUrl) != "https://cdn.example/kai.jpg" {
		t.Fatalf("creator = %+v", got.Creator)
	}
	if len(got.Members) != 1 {
		t.Fatalf("members = %d", len(got.Members))
	}
	member := got.Members[0]
	if member.UserId != f.user.ID.UUID() || member.Role != "creator" || !member.CanVote {
		t.Fatalf("member = %+v", member)
	}
	if !member.JoinedAt.Equal(f.clock.Now()) || deref(member.Handle) != "kai" || member.DisplayName != "Kai Cenat" {
		t.Fatalf("member = %+v", member)
	}
}

func TestPostCabal_usesTheDefaultSlippageWhenItIsOmitted(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	res, err := f.routes(nil).PostCabal(
		f.actor(t.Context()),
		cabalPost("c1", "Friends pot", "open", "all", "majority", domain.ExpiryDay, nil),
	)
	got := asCabal(t, res, err)
	if got.Rules.SlippageBps != domain.DefaultSlippageBps || f.wallets.Creates() != 1 {
		t.Fatalf("slippage = %d, creates %d", got.Rules.SlippageBps, f.wallets.Creates())
	}
}

func TestPostCabal_rejectsABadRequestBeforeAnyWallet(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	h := f.routes(nil)
	slippage := int32(0)
	cases := map[string]api.PostCabalRequestObject{
		"body":     {Params: api.PostCabalParams{IdempotencyKey: "c1"}},
		"name":     cabalPost("c1", "no", "request", "list", "unanimous", domain.ExpiryWeek, nil),
		"rules":    cabalPost("c1", "Friends pot", "request", "list", "unanimous", domain.ExpiryWeek, &slippage),
		"emptykey": cabalPost("", "Friends pot", "request", "list", "unanimous", domain.ExpiryWeek, nil),
	}
	for name, req := range cases {
		_, err := h.PostCabal(f.actor(t.Context()), req)
		cabals := f.count(t.Context(), t, "cabals")
		if errs.CodeOf(err) != errs.CodeInvalidInput || f.wallets.Creates() != 0 || cabals != 0 {
			t.Fatalf("%s: PostCabal = %v, creates %d, cabals %d", name, err, f.wallets.Creates(), cabals)
		}
	}
}

func TestPostCabal_returnsTheReadErrorAfterTheCabalExists(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	_, err := f.routes(failCards{}).PostCabal(
		f.actor(t.Context()),
		cabalPost("c1", "Friends pot", "request", "list", "unanimous", domain.ExpiryWeek, nil),
	)
	cabals := f.count(t.Context(), t, "cabals")
	eventsN := f.count(t.Context(), t, "events")
	if errs.CodeOf(err) != errs.CodeInternal || cabals != 1 || eventsN != 2 {
		t.Fatalf("PostCabal = %v, cabals %d, events %d", err, cabals, eventsN)
	}
}

func TestGetCabal_hidesTheInviteAndShowsAPendingRequest(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	seeded := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	setProfile(t, f, seeded.Creator.ID, "kai", "Kai Cenat", "https://cdn.example/kai.jpg")
	if _, err := f.pool.Exec(t.Context(), `UPDATE cabals SET picture_url = $2 WHERE id = $1`,
		seeded.ID.UUID(), "https://cdn.example/cabal.jpg"); err != nil {
		t.Fatal(err)
	}
	requestID := f.ids.NewV7()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO cabal_access_requests
		(id, cabal_id, user_id, direction, created_at) VALUES ($1, $2, $3, 'request', $4)`,
		requestID, seeded.ID.UUID(), f.user.ID.UUID(), f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	memberView := readCabal(t, f, seeded.Creator.ID, seeded.ID)
	outsider := readCabal(t, f, f.user.ID, seeded.ID)
	assertMemberView(t, seeded, memberView)
	assertOutsiderView(t, requestID, outsider)
}

func assertMemberView(t *testing.T, seeded testkit.SeededCabal, view api.Cabal) {
	t.Helper()
	if deref(view.PictureUrl) != "https://cdn.example/cabal.jpg" || view.Me == nil || view.Me.Role != "creator" {
		t.Fatalf("member view = %+v", view)
	}
	if view.InviteCode == nil || *view.InviteCode != seeded.InviteCode || view.MyAccessRequest != nil {
		t.Fatalf("member invite %v access %v", view.InviteCode, view.MyAccessRequest)
	}
	if view.TreasuryAddress != string(seeded.TreasuryAddress) || len(view.Members) != 2 {
		t.Fatalf("member view = %+v", view)
	}
	var plain api.CabalMember
	for _, member := range view.Members {
		if member.UserId == seeded.Members[1].ID.UUID() {
			plain = member
		}
	}
	assertPlainMember(t, view.Creator.Handle, plain, seeded.Members[1].ID.UUID())
}

func assertPlainMember(t *testing.T, handle *string, plain api.CabalMember, user uuid.UUID) {
	t.Helper()
	if deref(handle) != "kai" || plain.UserId != user || plain.DisplayName != "" {
		t.Fatalf("handle %v plain %+v", handle, plain)
	}
	if plain.Handle != nil || plain.PhotoUrl != nil {
		t.Fatalf("plain = %+v", plain)
	}
}

func assertOutsiderView(t *testing.T, requestID uuid.UUID, view api.Cabal) {
	t.Helper()
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if view.Me != nil || view.InviteCode != nil || view.MyAccessRequest == nil {
		t.Fatalf("outsider = %+v", view)
	}
	if view.MyAccessRequest.Id != requestID {
		t.Fatalf("access = %+v", view.MyAccessRequest)
	}
	if view.MyAccessRequest.Direction != "request" || view.MyAccessRequest.Status != "pending" {
		t.Fatalf("access = %+v", view.MyAccessRequest)
	}
	body := string(raw)
	if !json.Valid(raw) || !strings.Contains(body, `"invite_code":null`) || !strings.Contains(body, `"me":null`) {
		t.Fatalf("body = %s", body)
	}
}

func readCabal(t *testing.T, f createFixture, user ids.UserID, cabalID ids.CabalID) api.Cabal {
	t.Helper()
	ctx := auth.WithActor(t.Context(), auth.Actor{
		Kind: auth.ActorUser, ID: user.String(), Standing: auth.StandingActive,
	})
	res, err := f.routes(nil).GetCabal(ctx, api.GetCabalRequestObject{Id: cabalID.UUID()})
	got, ok := res.(api.GetCabal200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("GetCabal = %T, %v", res, err)
	}
	return api.Cabal(got)
}

func TestGetCabal_reportsAMissingCabalAndABrokenRead(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ddl  string
		code errs.Code
	}{
		{name: "missing", code: errs.CodeCabalNotFound},
		{name: "cabal", ddl: `ALTER TABLE cabals RENAME TO cabals_gone`, code: errs.CodeInternal},
		{name: "members", ddl: `ALTER TABLE cabal_members DROP COLUMN joined_at`, code: errs.CodeInternal},
		{name: "wallet", ddl: `DELETE FROM treasury_wallets`, code: errs.CodeInternal},
		{name: "access", ddl: `ALTER TABLE cabal_access_requests RENAME TO requests_gone`, code: errs.CodeInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			checkBrokenRead(t, tc.name, tc.ddl, tc.code)
		})
	}
}

func checkBrokenRead(t *testing.T, name, ddl string, code errs.Code) {
	t.Helper()
	f := newCreate(t)
	seeded := testkit.NewCabal(t, f.pool)
	if ddl != "" {
		if _, err := f.pool.Exec(t.Context(), ddl); err != nil {
			t.Fatal(err)
		}
	}
	id := seeded.ID
	if name == "missing" {
		id = ids.CabalIDFrom(f.ids.NewV7())
	}
	_, err := app.GetCabal(t.Context(), f.pool, f.routes(nil).Users, id, f.user.ID)
	if errs.CodeOf(err) != code {
		t.Fatalf("GetCabal = %v, want %s", err, code)
	}
	if name != "missing" {
		return
	}
	ctx := auth.WithActor(t.Context(), auth.Actor{
		Kind: auth.ActorUser, ID: f.user.ID.String(), Standing: auth.StandingActive,
	})
	_, httpErr := f.routes(nil).GetCabal(ctx, api.GetCabalRequestObject{Id: id.UUID()})
	if errs.CodeOf(httpErr) != errs.CodeCabalNotFound {
		t.Fatalf("HTTP GetCabal = %v", httpErr)
	}
}

func TestGetCabal_wrapsAUserLookupFailure(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	seeded := testkit.NewCabal(t, f.pool)
	_, err := app.GetCabal(t.Context(), f.pool, failCards{}, seeded.ID, f.user.ID)
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("GetCabal = %v", err)
	}
}

func TestHTTP_rejectsACallerThatIsNotAUser(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	h := f.routes(nil)
	req := cabalPost("c1", "Friends pot", "request", "list", "unanimous", domain.ExpiryWeek, nil)
	lookup := api.GetCabalRequestObject{Id: f.ids.NewV7()}
	rejectCaller(t.Context(), t, f, h, req, lookup, "absent", errs.CodeUnauthorized)
	agent := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorAgent, ID: f.user.ID.String()})
	rejectCaller(agent, t, f, h, req, lookup, "agent", errs.CodeForbidden)
	bad := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: "not-a-user"})
	rejectCaller(bad, t, f, h, req, lookup, "bad id", errs.CodeUnauthorized)
}

func rejectCaller(
	ctx context.Context, t *testing.T, f createFixture, h adapters.HTTP,
	req api.PostCabalRequestObject, lookup api.GetCabalRequestObject, name string, code errs.Code,
) {
	t.Helper()
	_, postErr := h.PostCabal(ctx, req)
	_, getErr := h.GetCabal(ctx, lookup)
	_, searchErr := h.GetCabals(ctx, api.GetCabalsRequestObject{})
	_, mineErr := h.GetMyCabals(ctx, api.GetMyCabalsRequestObject{})
	if errs.CodeOf(postErr) != code || errs.CodeOf(getErr) != code ||
		errs.CodeOf(searchErr) != code || errs.CodeOf(mineErr) != code || f.wallets.Creates() != 0 {
		t.Fatalf("%s: post %v get %v search %v mine %v creates %d",
			name, postErr, getErr, searchErr, mineErr, f.wallets.Creates())
	}
}

func setProfile(t *testing.T, f createFixture, id ids.UserID, handle, name, photo string) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(),
		`UPDATE users SET handle = $2, display_name = $3, photo_url = $4 WHERE id = $1`,
		id.UUID(), handle, name, photo)
	if err != nil {
		t.Fatal(err)
	}
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func TestSearchCabals_listsMatchesAndExcludesIneligibleCabals(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	member := testkit.NewCabal(t, f.pool, testkit.WithMembers(2), testkit.WithJoinMode("request"))
	banned := testkit.NewCabal(t, f.pool)
	empty := testkit.NewCabal(t, f.pool)
	nameCabal(t, f, member.ID.UUID(), "Friends Forever", "active")
	nameCabal(t, f, banned.ID.UUID(), "Friendly Banned", "banned")
	nameCabal(t, f, empty.ID.UUID(), "Friendly Empty", "active")
	emptyCabal(t, f, empty.ID.UUID())
	requestAccess(t, f, member.ID.UUID())
	got, err := searchCabals(t, f, f.user.ID, "  FRIEND  ", nil, nil)
	assertSearchMatch(t, got, err, member.ID.UUID())
	all, err := searchCabals(t, f, member.Creator.ID, "", nil, nil)
	assertSearchMember(t, all, err)
}

func assertSearchMatch(t *testing.T, got api.CabalSearchPage, err error, id uuid.UUID) {
	t.Helper()
	if err != nil || len(got.Items) != 1 || got.NextCursor != nil {
		t.Fatalf("GetCabals = %+v, %v", got, err)
	}
	item := got.Items[0]
	if item.Id != id || item.Name != "Friends Forever" || item.MemberCount != 2 || item.JoinMode != "request" ||
		item.IsMember || deref(item.MyAccessRequestStatus) != "pending" {
		t.Fatalf("item = %+v", item)
	}
}

func assertSearchMember(t *testing.T, got api.CabalSearchPage, err error) {
	t.Helper()
	if err != nil || len(got.Items) != 1 || !got.Items[0].IsMember {
		t.Fatalf("empty GetCabals = %+v, %v", got, err)
	}
}

func nameCabal(t *testing.T, f createFixture, id uuid.UUID, name, status string) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(), `UPDATE cabals SET name = $2, status = $3 WHERE id = $1`, id, name, status)
	if err != nil {
		t.Fatal(err)
	}
}

func emptyCabal(t *testing.T, f createFixture, id uuid.UUID) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), `DELETE FROM cabal_members WHERE cabal_id = $1`, id); err != nil {
		t.Fatal(err)
	}
}

func requestAccess(t *testing.T, f createFixture, cabalID uuid.UUID) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO cabal_access_requests
		(id, cabal_id, user_id, direction, status, created_at)
		VALUES ($1, $2, $3, 'request', 'pending', now())`, f.ids.NewV7(), cabalID, f.user.ID.UUID())
	if err != nil {
		t.Fatal(err)
	}
}

func TestSearchCabals_refusesInvalidQueryLimitAndCursor(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	for _, request := range []api.GetCabalsRequestObject{
		{Params: api.GetCabalsParams{Query: ptr("x")}},
		{Params: api.GetCabalsParams{Limit: ptr(0)}},
		{Params: api.GetCabalsParams{Limit: ptr(51)}},
		{Params: api.GetCabalsParams{Cursor: ptr("!")}},
		{Params: api.GetCabalsParams{Cursor: ptr(cabalCursor("not json"))}},
		{Params: api.GetCabalsParams{Cursor: ptr(cabalCursor(`{"member_count":1,"created_at":"bad","id":"bad"}`))}},
		{Params: api.GetCabalsParams{Cursor: ptr(cabalCursor(`{"member_count":0,"created_at":"2026-10-02T15:00:00Z","id":"bad"}`))}},
	} {
		_, err := f.routes(nil).GetCabals(f.actor(t.Context()), request)
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("GetCabals = %v, want invalid input", err)
		}
	}
}

func TestSearchCabals_reportsAnUnavailableRead(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE cabals RENAME TO cabals_gone`); err != nil {
		t.Fatal(err)
	}
	_, err := f.routes(nil).GetCabals(f.actor(t.Context()), api.GetCabalsRequestObject{})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("GetCabals = %v, want internal", err)
	}
}

func TestSearchCabals_cursorWalksEveryRowOnce(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	created := seedTiedCabals(t, f)
	limit := 2
	seen := make(map[uuid.UUID]struct{})
	var cursor *string
	for {
		page, err := searchCabals(t, f, f.user.ID, "", &limit, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			if _, exists := seen[item.Id]; exists {
				t.Fatalf("cabal %s appeared twice", item.Id)
			}
			seen[item.Id] = struct{}{}
		}
		if page.NextCursor == nil {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != len(created) {
		t.Fatalf("saw %d cabals, want %d", len(seen), len(created))
	}
	assertAllCabalsSeen(t, created, seen)
}

func seedTiedCabals(t *testing.T, f createFixture) map[uuid.UUID]struct{} {
	t.Helper()
	created := make(map[uuid.UUID]struct{})
	createdAt := time.Date(2026, 10, 2, 16, 0, 0, 0, time.UTC)
	for range 5 {
		c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
		if _, err := f.pool.Exec(
			t.Context(), `UPDATE cabals SET created_at = $2 WHERE id = $1`, c.ID.UUID(), createdAt,
		); err != nil {
			t.Fatal(err)
		}
		created[c.ID.UUID()] = struct{}{}
	}
	return created
}

func assertAllCabalsSeen(t *testing.T, want, got map[uuid.UUID]struct{}) {
	t.Helper()
	for id := range want {
		if _, ok := got[id]; !ok {
			t.Fatalf("missing cabal %s", id)
		}
	}
}

func TestSearchCabals_routerAcceptsAnEmptyQuery(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	eligible := testkit.NewCabal(t, f.pool)
	h, verifier := f.router(t)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/cabals?query=", nil)
	req.Header.Set("Authorization", "Bearer "+verifier.Mint(f.user.ID.String(), f.clock.Now().Add(time.Hour)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/cabals?query= = %d %s, want 200", rec.Code, rec.Body)
	}
	var page api.CabalSearchPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode response %s: %v", rec.Body, err)
	}
	if len(page.Items) != 1 || page.Items[0].Id != eligible.ID.UUID() {
		t.Fatalf("response = %+v, want cabal %s", page, eligible.ID.UUID())
	}
}

func TestSearchCabals_usesTheTrigramIndex(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	conn, err := f.pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(t.Context(), `SET enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(t.Context(), `RESET enable_seqscan`) }()
	rows, err := conn.Query(t.Context(), `EXPLAIN SELECT c.id, c.name, c.picture_url, c.join_mode, c.created_at,
  (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)::int AS member_count,
  EXISTS (SELECT 1 FROM cabal_members m WHERE m.cabal_id = c.id AND m.user_id = $1) AS is_member,
  r.status AS my_access_request_status
FROM cabals c
LEFT JOIN cabal_access_requests r ON r.cabal_id = c.id AND r.user_id = $1
  AND r.direction = 'request' AND r.status = 'pending'
WHERE c.status <> 'banned'
  AND ($2::text = '' OR lower(c.name) LIKE '%' || lower($2::text) || '%')
  AND (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id) > 0
  AND (
    $3::int IS NULL
    OR (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)::int < $3::int
    OR (
      (SELECT count(*) FROM cabal_members m WHERE m.cabal_id = c.id)::int = $3::int
      AND (c.created_at, c.id) < ($4::timestamptz, $5::uuid)
    )
  )
ORDER BY member_count DESC, c.created_at DESC, c.id DESC
LIMIT $6::int`, f.user.ID.UUID(), "fri", nil, nil, nil, int32(21))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line)
	}
	if err := rows.Err(); err != nil || !strings.Contains(plan.String(), "cabals_name_trgm_idx") {
		t.Fatalf("plan = %q, %v", plan.String(), err)
	}
}

func TestMyCabals_listsMembershipsAndCreatorRequests(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	creator := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	member := testkit.NewCabal(t, f.pool)
	requestAccess(t, f, creator.ID.UUID())
	joinMyCabal(t, f, member.ID.UUID(), time.Date(2026, 10, 2, 16, 0, 0, 0, time.UTC))
	page, err := f.routes(nil).GetMyCabals(auth.WithActor(t.Context(), auth.Actor{
		Kind: auth.ActorUser, ID: creator.Creator.ID.String(), Standing: auth.StandingActive,
	}), api.GetMyCabalsRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	items := []api.MyCabal(page.(api.GetMyCabals200JSONResponse))
	if len(items) != 1 || items[0].PendingRequestCount != 1 || items[0].Role != "creator" {
		t.Fatalf("creator cabals = %+v", items)
	}
	mine, err := f.routes(nil).GetMyCabals(f.actor(t.Context()), api.GetMyCabalsRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	items = []api.MyCabal(mine.(api.GetMyCabals200JSONResponse))
	if len(items) != 1 || items[0].Id != member.ID.UUID() || items[0].PendingRequestCount != 0 {
		t.Fatalf("my cabals = %+v", items)
	}
}

func TestMyCabals_returnsAnEmptyListForANewUser(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	result, err := f.routes(nil).GetMyCabals(f.actor(t.Context()), api.GetMyCabalsRequestObject{})
	items, ok := result.(api.GetMyCabals200JSONResponse)
	if err != nil || !ok || len(items) != 0 {
		t.Fatalf("GetMyCabals = %v, %v", result, err)
	}
}

func TestMyCabals_ordersByJoinedAtDescending(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	oldest, middle, newest := testkit.NewCabal(t, f.pool), testkit.NewCabal(t, f.pool), testkit.NewCabal(t, f.pool)
	joinMyCabal(t, f, oldest.ID.UUID(), now.Add(-2*time.Hour))
	joinMyCabal(t, f, middle.ID.UUID(), now.Add(-time.Hour))
	joinMyCabal(t, f, newest.ID.UUID(), now)
	result, err := f.routes(nil).GetMyCabals(f.actor(t.Context()), api.GetMyCabalsRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	items := []api.MyCabal(result.(api.GetMyCabals200JSONResponse))
	if len(items) != 3 || items[0].Id != newest.ID.UUID() || items[1].Id != middle.ID.UUID() ||
		items[2].Id != oldest.ID.UUID() {
		t.Fatalf("cabals = %+v", items)
	}
}

func TestMyCabals_reportsAnUnavailableRead(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE cabal_members RENAME TO cabal_members_gone`); err != nil {
		t.Fatal(err)
	}
	_, err := f.routes(nil).GetMyCabals(f.actor(t.Context()), api.GetMyCabalsRequestObject{})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("GetMyCabals = %v, want internal", err)
	}
}

func TestSearchCabals_threeCharacterQueryIsUnderTwentyMillisecondsP95(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	seedSearchPerfCabals(t, f)
	q := sqlc.New(f.pool)
	durations := make([]time.Duration, 60)
	for i := range durations {
		started := clock.Real{}.Now()
		if _, err := q.SearchCabals(t.Context(), sqlc.SearchCabalsParams{
			ActorID: f.user.ID.UUID(), Query: "abc", PageSize: 21,
		}); err != nil {
			t.Fatal(err)
		}
		durations[i] = clock.Real{}.Now().Sub(started)
	}
	slices.Sort(durations)
	p95 := durations[56]
	t.Logf("search p95 = %s", p95)
	if p95 >= 20*time.Millisecond {
		t.Fatalf("search p95 = %s, want under 20ms", p95)
	}
}

func seedSearchPerfCabals(t *testing.T, f createFixture) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(), `WITH seeded AS (
		INSERT INTO cabals (id, name, creator_id, join_mode, voter_mode, threshold, proposal_expiry_seconds,
			slippage_bps, invite_code, created_at, updated_at)
		SELECT md5(n::text)::uuid, CASE WHEN n <= 14 THEN 'abc cabal ' || n ELSE 'other cabal ' || n END,
			$1, 'open', 'all', 'majority', 86400, 100,
			lpad(n::text, 10, '0'), now(), now() FROM generate_series(1, 10000) n RETURNING id
	) INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
	SELECT id, $1, 'creator', true, now() FROM seeded`, f.user.ID.UUID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `ANALYZE cabals, cabal_members`); err != nil {
		t.Fatal(err)
	}
}

func joinMyCabal(t *testing.T, f createFixture, cabalID uuid.UUID, joinedAt time.Time) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
		VALUES ($1, $2, 'member', false, $3)`, cabalID, f.user.ID.UUID(), joinedAt)
	if err != nil {
		t.Fatal(err)
	}
}

func searchCabals(
	t *testing.T, f createFixture, user ids.UserID, query string, limit *int, cursor *string,
) (api.CabalSearchPage, error) {
	t.Helper()
	ctx := auth.WithActor(t.Context(), auth.Actor{
		Kind: auth.ActorUser, ID: user.String(), Standing: auth.StandingActive,
	})
	result, err := f.routes(nil).GetCabals(ctx, api.GetCabalsRequestObject{
		Params: api.GetCabalsParams{Query: &query, Limit: limit, Cursor: cursor},
	})
	if err != nil {
		return api.CabalSearchPage{}, err
	}
	page, ok := result.(api.GetCabals200JSONResponse)
	if !ok {
		t.Fatalf("GetCabals = %T", result)
	}
	return api.CabalSearchPage(page), nil
}

func ptr[T any](value T) *T { return &value }

func cabalCursor(value string) string { return base64.RawURLEncoding.EncodeToString([]byte(value)) }
