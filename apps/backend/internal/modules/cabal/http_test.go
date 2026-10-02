package cabal_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
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
	if errs.CodeOf(postErr) != code || errs.CodeOf(getErr) != code || f.wallets.Creates() != 0 {
		t.Fatalf("%s: post %v get %v creates %d", name, postErr, getErr, f.wallets.Creates())
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
