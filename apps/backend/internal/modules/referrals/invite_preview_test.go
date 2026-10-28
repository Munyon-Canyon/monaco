package referrals_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/referralsapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const andresPhoto = "https://img.example/andres.png"

type previewMember struct {
	id       ids.UserID
	name     string
	joinedAt time.Time
}

type previewWorld struct {
	cabal   ids.CabalID
	inviter ids.UserID
	members []previewMember
	created time.Time
}

func (s server) preview(t *testing.T, code string, cabal ids.CabalID) *httptest.ResponseRecorder {
	t.Helper()
	path := fmt.Sprintf("/v1/referrals/%s/cabals/%s", code, cabal)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func previewBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	dec := json.NewDecoder(rec.Body)
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return body
}

func seedCabalRow(t *testing.T, pool *pgxpool.Pool, name, picture, status string, created time.Time) ids.CabalID {
	t.Helper()
	creator := testkit.SeedUser(t, pool, testkit.UserOpts{})
	id := ids.CabalIDFrom(ids.Real{}.NewV7())
	if _, err := pool.Exec(t.Context(), `INSERT INTO cabals (id, name, picture_url, creator_id, join_mode, voter_mode,
		threshold, proposal_expiry_seconds, status, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, 'request', 'all', 'majority', 86400, $5, $6, $6)`,
		id.UUID(), name, picture, creator.ID.UUID(), status, created); err != nil {
		t.Fatal(err)
	}
	return id
}

func joinCabal(t *testing.T, pool *pgxpool.Pool, cabal ids.CabalID, user ids.UserID, joinedAt time.Time) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
		VALUES ($1, $2, 'member', true, $3)`, cabal.UUID(), user.UUID(), joinedAt); err != nil {
		t.Fatal(err)
	}
}

func seedPreviewWorld(t *testing.T, s server, others int) previewWorld {
	t.Helper()
	created := time.Date(2026, 8, 14, 9, 30, 0, 0, time.UTC)
	w := previewWorld{
		cabal: seedCabalRow(t, s.pool, "AC Capital", "https://img.example/ac.png", "active", created), created: created,
	}
	w.inviter = seedOwner(t, s.pool, owner{
		handle: "andres", code: "k7m4qx2p", unlocked: true, name: "Andres", photo: andresPhoto,
	})
	joinCabal(t, s.pool, w.cabal, w.inviter, created)
	for i := range others {
		name := fmt.Sprintf("Member %d", i)
		id := seedOwner(t, s.pool, owner{handle: fmt.Sprintf("member%d", i), name: name})
		joined := created.Add(time.Duration(i+1) * time.Hour)
		joinCabal(t, s.pool, w.cabal, id, joined)
		w.members = append(w.members, previewMember{id: id, name: name, joinedAt: joined})
	}
	return w
}

func TestInvitePreview_aMembersCodeGivesTheCabalsPublicFacts(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	w := seedPreviewWorld(t, s, 3)
	rec := s.preview(t, "K7M4QX2P", w.cabal)
	first := make([]any, 0, len(w.members))
	for _, m := range w.members {
		first = append(first, map[string]any{"display_name": m.name, "photo_url": nil})
	}
	deps := module.Deps{Pool: s.pool, Clock: s.clock}
	pot, err := treasury.New(deps).Queries().PotValue(t.Context(), w.cabal)
	if err != nil {
		t.Fatal(err)
	}
	view, err := cabal.New(deps).Queries().Cabal(t.Context(), w.cabal)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"inviter": map[string]any{"display_name": "Andres", "photo_url": andresPhoto},
		"cabal": map[string]any{
			"id": w.cabal.String(), "name": view.Name, "picture_url": "https://img.example/ac.png",
			"member_count": json.Number(strconv.Itoa(view.MemberCount)), "since": view.CreatedAt.Format(time.DateOnly),
			"join_mode": "request", "first_members": first, "pot_value_micros": pot.String(),
		},
		"return_1m_bps": nil, "turnout_month_bps": nil, "open_proposal": nil,
	}
	if got := previewBody(t, rec); rec.Code != http.StatusOK || !reflect.DeepEqual(got, want) {
		t.Fatalf("GET preview = %d %v, want 200 %v", rec.Code, got, want)
	}
	if view.MemberCount != 4 || view.CreatedAt.Format(time.DateOnly) != "2026-08-14" {
		t.Fatalf("getCabal facts = %d members since %s, want 4 since 2026-08-14", view.MemberCount, view.CreatedAt)
	}
}

func TestInvitePreview_everyMissIsTheSame200WithNoCabal(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	w := seedPreviewWorld(t, s, 2)
	seedOwner(t, s.pool, owner{handle: "outsider", code: "p7m4qx2p", unlocked: true, name: "Olu"})
	seedOwner(t, s.pool, owner{handle: "lockedkai", name: "Kai"})
	banned := seedCabalRow(t, s.pool, "Banned pot", "", "banned", w.created)
	joinCabal(t, s.pool, banned, w.inviter, w.created)
	for name, tt := range map[string]struct {
		code    string
		cabal   ids.CabalID
		inviter any
	}{
		"non-member":      {"p7m4qx2p", w.cabal, map[string]any{"display_name": "Olu", "photo_url": nil}},
		"banned cabal":    {"k7m4qx2p", banned, map[string]any{"display_name": "Andres", "photo_url": andresPhoto}},
		"unknown cabal":   {"k7m4qx2p", ids.CabalIDFrom(ids.Real{}.NewV7()), map[string]any{"display_name": "Andres", "photo_url": andresPhoto}},
		"unknown code":    {"zzzzzzzz", w.cabal, nil},
		"locked handle":   {"lockedkai", w.cabal, nil},
		"unknown handle":  {"nobody", w.cabal, nil},
		"handle of owner": {"andres", w.cabal, map[string]any{"display_name": "Andres", "photo_url": andresPhoto}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := s.preview(t, tt.code, tt.cabal)
			body := previewBody(t, rec)
			if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-cache" {
				t.Fatalf("GET = %d Cache-Control %q, want 200 no-cache", rec.Code, rec.Header().Get("Cache-Control"))
			}
			if name == "handle of owner" {
				if body["cabal"] == nil {
					t.Fatalf("body = %v, want the cabal: a member's unlocked handle works like the code", body)
				}
				return
			}
			want := map[string]any{
				"inviter":           tt.inviter,
				"cabal":             nil,
				"return_1m_bps":     nil,
				"turnout_month_bps": nil,
				"open_proposal":     nil,
			}
			if !reflect.DeepEqual(body, want) {
				t.Fatalf("body = %v, want %v", body, want)
			}
		})
	}
}

func TestInvitePreview_firstMembersSkipTheInviterAndStopAtFourInJoinOrder(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	w := seedPreviewWorld(t, s, 6)
	tied := w.members[3].joinedAt
	for _, m := range w.members[4:] {
		if _, err := s.pool.Exec(t.Context(),
			`UPDATE cabal_members SET joined_at = $3 WHERE cabal_id = $1 AND user_id = $2`,
			w.cabal.UUID(), m.id.UUID(), tied); err != nil {
			t.Fatal(err)
		}
	}
	w.members[4].joinedAt, w.members[5].joinedAt = tied, tied
	got := peopleNames(t, s.preview(t, "k7m4qx2p", w.cabal))
	want := []string{w.members[0].name, w.members[1].name, w.members[2].name, w.members[3].name}
	if !slices.Equal(got, want) {
		t.Fatalf("first_members = %v, want %v", got, want)
	}
	if _, err := s.pool.Exec(t.Context(),
		`UPDATE cabal_members SET joined_at = $2 WHERE cabal_id = $1 AND user_id <> $3`,
		w.cabal.UUID(), w.created.Add(time.Hour), w.inviter.UUID()); err != nil {
		t.Fatal(err)
	}
	byID := slices.Clone(w.members)
	slices.SortFunc(byID, func(a, b previewMember) int { return strings.Compare(a.id.String(), b.id.String()) })
	got = peopleNames(t, s.preview(t, "k7m4qx2p", w.cabal))
	want = []string{byID[0].name, byID[1].name, byID[2].name, byID[3].name}
	if !slices.Equal(got, want) {
		t.Fatalf("first_members of a six-way tie = %v, want the four lowest user ids %v", got, want)
	}
}

func peopleNames(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	cabalBody, _ := previewBody(t, rec)["cabal"].(map[string]any)
	people, _ := cabalBody["first_members"].([]any)
	names := make([]string, 0, len(people))
	for _, p := range people {
		names = append(names, p.(map[string]any)["display_name"].(string))
	}
	return names
}

func keyProblems(path string, v any) []string {
	var problems []string
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			key := strings.TrimPrefix(path+"."+k, ".")
			problems = append(problems, nameProblems(key, k)...)
			problems = append(problems, keyProblems(key, child)...)
		}
	case []any:
		for _, child := range v {
			problems = append(problems, keyProblems(path+"[]", child)...)
		}
	}
	return problems
}

func nameProblems(key, name string) []string {
	var problems []string
	for _, part := range []string{"user_id", "handle", "share", "side", "choice", "ballot", "slice", "stake"} {
		if strings.Contains(name, part) {
			problems = append(problems, fmt.Sprintf("key %s holds %q", key, part))
		}
	}
	if strings.HasSuffix(name, "id") && key != "cabal.id" && key != "open_proposal.id" {
		problems = append(problems, fmt.Sprintf("key %s is an id the public preview must not carry", key))
	}
	return problems
}

func TestInvitePreview_theJSONCarriesNoIdentityShareOrVoteSide(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	w := seedPreviewWorld(t, s, 4)
	rec := s.preview(t, "k7m4qx2p", w.cabal)
	for _, problem := range keyProblems("", previewBody(t, rec)) {
		t.Error(problem)
	}
	raw := rec.Body.String()
	if strings.Contains(raw, w.inviter.String()) || strings.Contains(raw, "andres\"") {
		t.Errorf("body %s holds the inviter's id or handle", raw)
	}
	for _, m := range w.members {
		if strings.Contains(raw, m.id.String()) {
			t.Errorf("body %s holds member id %s", raw, m.id)
		}
	}
}

func TestInvitePreview_isNeverCachedSoAJoinShowsOnTheNextRead(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	w := seedPreviewWorld(t, s, 2)
	count := func() json.Number {
		t.Helper()
		rec := s.preview(t, "k7m4qx2p", w.cabal)
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("Cache-Control = %q, want no-cache", got)
		}
		cabalBody, _ := previewBody(t, rec)["cabal"].(map[string]any)
		return cabalBody["member_count"].(json.Number)
	}
	before := count()
	joinCabal(
		t,
		s.pool,
		w.cabal,
		seedOwner(t, s.pool, owner{handle: "latecomer", name: "Late"}),
		w.created.Add(48*time.Hour),
	)
	if after := count(); before != "3" || after != "4" {
		t.Fatalf("member_count before and after a join = %v and %v, want 3 and 4", before, after)
	}
}

func TestInvitePreview_theSixtyFirstCallFromOneAddressIsRateLimited(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	w := seedPreviewWorld(t, s, 1)
	for i := range 60 {
		if rec := s.preview(t, "k7m4qx2p", w.cabal); rec.Code != http.StatusOK {
			t.Fatalf("call %d = %d %s, want 200", i+1, rec.Code, rec.Body)
		}
	}
	expectRateLimited(t, s.preview(t, "k7m4qx2p", w.cabal))
}

type fakeWorld struct {
	cabal    ids.CabalID
	inviter  ids.UserID
	friend   ids.UserID
	now      time.Time
	identity *fakes.Identity
	cabals   *fakes.Cabal
	pots     *fakes.Treasury
	query    *app.InvitePreviewQuery
}

func newFakeWorld(t *testing.T, deleted bool) fakeWorld {
	t.Helper()
	gen := testkit.NewIDs(7)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	w := fakeWorld{
		cabal: ids.CabalIDFrom(gen.NewV7()), inviter: ids.NewUserID(gen), friend: ids.NewUserID(gen), now: now,
	}
	friend := w.friend
	card := func(id ids.UserID, handle string, gone bool) identity.UserCard {
		return identity.UserCard{
			ID: id, Handle: handle, DisplayName: strings.ToUpper(handle[:1]) + handle[1:],
			AccountStatus: identity.AccountActive, FirstDepositAt: &now, Deleted: gone,
		}
	}
	w.identity = fakes.NewIdentity(
		[]identity.UserCard{card(w.inviter, "andres", false), card(friend, "mia", deleted)}, nil)
	w.cabals = fakes.NewCabal(
		[]fakes.CabalSeed{
			{View: cabal.View{ID: w.cabal, Name: "AC Capital", CreatedAt: now, JoinMode: cabal.JoinOpen}},
		},
		[]fakes.CabalMember{
			{CabalID: w.cabal, Member: cabal.MemberView{UserID: w.inviter, Role: cabal.RoleCreator, JoinedAt: now}},
			{CabalID: w.cabal, Member: cabal.MemberView{UserID: friend, Role: cabal.RoleMember, JoinedAt: now}},
		},
	)
	w.pots = fakes.NewTreasury()
	w.query = app.NewInvitePreviewQuery(app.InvitePreviewDeps{
		Resolver: app.Resolver{Users: w.identity}, Cabals: w.cabals, Pots: w.pots,
	})
	return w
}

func TestInvitePreviewQuery_aDeletedMemberIsLeftOutOfFirstMembers(t *testing.T) {
	t.Parallel()
	w := newFakeWorld(t, true)
	got, err := w.query.Run(t.Context(), "andres", w.cabal)
	if err != nil || got.Cabal == nil || len(got.Cabal.FirstMembers) != 0 || got.Cabal.MemberCount != 2 {
		t.Fatalf("Run = %+v, %v, want the cabal with two members and no first members", got, err)
	}
}

func TestInvitePreviewQuery_everyReadFailureSurfacesUnchanged(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test.read")
	for name, arrange := range map[string]func(w fakeWorld){
		"resolving the code":  func(w fakeWorld) { w.identity.Fail("UserByHandle", boom) },
		"reading the inviter": func(w fakeWorld) { w.identity.Fail("UsersByID", boom) },
		"reading the cabal":   func(w fakeWorld) { w.cabals.Fail("InvitePreview", boom) },
		"reading the members": func(w fakeWorld) {
			w.identity.FailOnce("UsersByID", nil)
			w.identity.FailOnce("UsersByID", boom)
		},
		"reading the pot": func(w fakeWorld) { w.pots.Fail("PotValue", boom) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newFakeWorld(t, false)
			arrange(w)
			if _, err := w.query.Run(t.Context(), "andres", w.cabal); !errors.Is(err, boom) {
				t.Fatalf("Run err = %v, want %v", err, boom)
			}
			arrange(w)
			_, httpErr := adapters.HTTP{Cabals: w.query}.GetReferralCabal(
				t.Context(), api.GetReferralCabalRequestObject{Code: "andres", CabalId: w.cabal.UUID()})
			if !errors.Is(httpErr, boom) {
				t.Fatalf("GetReferralCabal err = %v, want %v", httpErr, boom)
			}
		})
	}
}

func TestGetReferralCabal_aCabalIDThatIsNotV7IsInvalidInput(t *testing.T) {
	t.Parallel()
	w := newFakeWorld(t, false)
	_, err := adapters.HTTP{Cabals: w.query}.GetReferralCabal(
		t.Context(),
		api.GetReferralCabalRequestObject{
			Code: "andres",
			CabalId: uuid.UUID{
				0x5f,
				0x0f,
				0x6c,
				0x2e,
				0x8a,
				0x3b,
				0x4d,
				0x1e,
				0x9b,
				0x7a,
				0x2c,
				0x4d,
				0x6e,
				0x8f,
				0x1a,
				0x3b,
			},
		},
	)
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("GetReferralCabal err = %v, want invalid_input", err)
	}
}

type factsFake struct {
	open      governanceport.OpenProposal
	hasOpen   bool
	voted     int
	eligible  int
	from      time.Time
	openErr   error
	turnedErr error
}

func (f *factsFake) NewestOpenProposal(context.Context, ids.CabalID) (governanceport.OpenProposal, bool, error) {
	return f.open, f.hasOpen, f.openErr
}

func (f *factsFake) MonthTurnout(_ context.Context, _ ids.CabalID, from time.Time) (int, int, error) {
	f.from = from
	return f.voted, f.eligible, f.turnedErr
}

type returnsFake struct {
	bps *int64
	err error
}

func (r returnsFake) CabalReturns(
	_ context.Context, rng rankingport.Range, cabals []ids.CabalID,
) (map[ids.CabalID]rankingport.CabalReturn, error) {
	if rng != rankingport.Range1M {
		return nil, errs.New(errs.CodeInternal, "test.range")
	}
	if r.err != nil {
		return nil, r.err
	}
	return map[ids.CabalID]rankingport.CabalReturn{cabals[0]: {ReturnBps: r.bps}}, nil
}

type repliesFake struct {
	counts map[ids.ProposalID]int
	err    error
}

func (r repliesFake) ReplyCounts(context.Context, []ids.ProposalID) (map[ids.ProposalID]int, error) {
	return r.counts, r.err
}

func (w fakeWorld) monthDeps(facts *factsFake, bps *int64, replies repliesFake) app.InvitePreviewDeps {
	return app.InvitePreviewDeps{
		Resolver: app.Resolver{Users: w.identity}, Cabals: w.cabals, Pots: w.pots, Clock: testkit.NewClock(w.now),
		Votes: facts, Returns: returnsFake{bps: bps}, Assets: marketfake.NewCatalog(marketfake.Fixtures()...),
		Replies: replies,
	}
}

func (w fakeWorld) monthQuery(facts *factsFake, bps *int64, replies repliesFake) *app.InvitePreviewQuery {
	return app.NewInvitePreviewQuery(w.monthDeps(facts, bps, replies))
}

func (w fakeWorld) nvda() governanceport.OpenProposal {
	usdc := uint64(400_000_000)
	return governanceport.OpenProposal{
		ID: ids.ProposalIDFrom(ids.Real{}.NewV7()), Kind: "buy", Symbol: "AAPLx",
		Mint:       marketfake.AAPLx().Mint.String(),
		USDCMicros: &usdc, ProposerID: w.friend, Thesis: "small add before earnings\nsecond line", Yes: 4, No: 1,
		Voted: 5, Eligible: 8, ExpiresAt: w.now.Add(4 * time.Hour),
	}
}

func bpsIs(t *testing.T, name string, got *int64, want int64) {
	t.Helper()
	if got == nil || *got != want {
		t.Errorf("%s = %v, want %d", name, got, want)
	}
}

func TestInvitePreviewQuery_composesTheMonthFactsTheFrameDraws(t *testing.T) {
	t.Parallel()
	w := newFakeWorld(t, false)
	open := w.nvda()
	loss := int64(-410)
	facts := &factsFake{open: open, hasOpen: true, voted: 47, eligible: 50}
	replies := repliesFake{counts: map[ids.ProposalID]int{open.ID: 6}}

	got, err := w.monthQuery(facts, &loss, replies).Run(t.Context(), "andres", w.cabal)
	if err != nil || got.Cabal == nil {
		t.Fatalf("Run = %+v, %v, want the cabal", got, err)
	}
	bpsIs(t, "Return1MBps", got.Cabal.Return1MBps, -410)
	bpsIs(t, "TurnoutBps", got.Cabal.TurnoutBps, 9400)
	if want := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC); !facts.from.Equal(want) {
		t.Errorf("turnout counted from %s, want the first instant of the UTC month %s", facts.from, want)
	}
	usdc, sixReplies := uint64(400_000_000), 6
	want := &app.PreviewProposal{
		ID: open.ID, Kind: "buy", Symbol: "AAPLx", AssetName: "Apple", AssetLogoURL: marketfake.AAPLx().LogoURL,
		USDCMicros: &usdc, ProposerName: "Mia", Title: "small add before earnings", Yes: 4, No: 1, Voted: 5,
		Eligible: 8, Replies: &sixReplies, ExpiresAt: open.ExpiresAt,
	}
	if !reflect.DeepEqual(got.Cabal.OpenProposal, want) {
		t.Errorf("OpenProposal = %+v, want %+v", got.Cabal.OpenProposal, want)
	}
}

func TestInvitePreviewQuery_theMonthFactsAreNullWhenThereIsNothingToShow(t *testing.T) {
	t.Parallel()
	w := newFakeWorld(t, false)
	got, err := w.monthQuery(&factsFake{}, nil, repliesFake{}).Run(t.Context(), "andres", w.cabal)
	if err != nil || got.Cabal == nil {
		t.Fatalf("Run = %+v, %v, want the cabal", got, err)
	}
	if c := got.Cabal; c.Return1MBps != nil || c.TurnoutBps != nil || c.OpenProposal != nil {
		t.Errorf("month facts = %v %v %v, want all null with nothing eligible, ranked or open",
			c.Return1MBps, c.TurnoutBps, c.OpenProposal)
	}
	zero := int64(0)
	facts := &factsFake{voted: 0, eligible: 3}
	got, err = w.monthQuery(facts, &zero, repliesFake{}).Run(t.Context(), "andres", w.cabal)
	if err != nil || got.Cabal.TurnoutBps == nil || *got.Cabal.TurnoutBps != 0 ||
		got.Cabal.Return1MBps == nil || *got.Cabal.Return1MBps != 0 {
		t.Fatalf("Run = %+v, %v, want 0 turnout and 0 return, not null, when ballots were eligible", got.Cabal, err)
	}
}

func TestInvitePreviewQuery_aFailingReplyCountGivesNullRepliesNotAnError(t *testing.T) {
	t.Parallel()
	w := newFakeWorld(t, false)
	facts := &factsFake{open: w.nvda(), hasOpen: true, voted: 5, eligible: 8}
	replies := repliesFake{err: errs.New(errs.CodeInternal, "test.replies")}
	got, err := w.monthQuery(facts, nil, replies).Run(t.Context(), "andres", w.cabal)
	if err != nil || got.Cabal == nil || got.Cabal.OpenProposal == nil || got.Cabal.OpenProposal.Replies != nil {
		t.Fatalf("Run = %+v, %v, want the proposal with replies null", got.Cabal, err)
	}
	if got.Cabal.OpenProposal.Yes != 4 || got.Cabal.TurnoutBps == nil {
		t.Errorf("Run = %+v, want the rest of the preview intact", got.Cabal)
	}
}

func TestInvitePreviewQuery_aSellHasNoUSDCAndAnEmptyThesisAnEmptyTitle(t *testing.T) {
	t.Parallel()
	w := newFakeWorld(t, false)
	sell := w.nvda()
	sell.Kind, sell.USDCMicros, sell.Thesis = "sell", nil, ""
	facts := &factsFake{open: sell, hasOpen: true, voted: 1, eligible: 2}
	query := w.monthQuery(facts, nil, repliesFake{counts: map[ids.ProposalID]int{}})
	got, err := query.Run(t.Context(), "andres", w.cabal)
	if err != nil || got.Cabal == nil || got.Cabal.OpenProposal == nil {
		t.Fatalf("Run = %+v, %v, want the sell", got.Cabal, err)
	}
	p := got.Cabal.OpenProposal
	if p.Kind != "sell" || p.USDCMicros != nil || p.Title != "" || p.Replies == nil || *p.Replies != 0 {
		t.Errorf("OpenProposal = %+v, want a sell with no USDC, an empty title and 0 replies", p)
	}
	body, err := adapters.HTTP{Cabals: w.monthQuery(facts, nil, repliesFake{counts: map[ids.ProposalID]int{}})}.
		GetReferralCabal(t.Context(), api.GetReferralCabalRequestObject{Code: "andres", CabalId: w.cabal.UUID()})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(body.(api.GetReferralCabal200JSONResponse).Body.OpenProposal)
	if err != nil || !strings.Contains(string(raw), `"usdc_micros":null`) {
		t.Errorf("open_proposal JSON = %s, %v, want usdc_micros null", raw, err)
	}
}

func TestInvitePreviewQuery_everyMonthReadFailureSurfacesUnchanged(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test.month")
	for name, arrange := range map[string]func(w fakeWorld, deps *app.InvitePreviewDeps, facts *factsFake){
		"reading the return": func(_ fakeWorld, deps *app.InvitePreviewDeps, _ *factsFake) {
			deps.Returns = returnsFake{err: boom}
		},
		"reading the turnout":       func(_ fakeWorld, _ *app.InvitePreviewDeps, facts *factsFake) { facts.turnedErr = boom },
		"reading the open proposal": func(_ fakeWorld, _ *app.InvitePreviewDeps, facts *factsFake) { facts.openErr = boom },
		"parsing the mint": func(_ fakeWorld, _ *app.InvitePreviewDeps, facts *factsFake) {
			facts.open.Mint = "not a mint"
		},
		"reading the asset": func(_ fakeWorld, deps *app.InvitePreviewDeps, _ *factsFake) {
			deps.Assets = marketfake.NewCatalog()
		},
		"reading the proposer": func(w fakeWorld, _ *app.InvitePreviewDeps, _ *factsFake) {
			w.identity.FailOnce("UsersByID", nil)
			w.identity.FailOnce("UsersByID", nil)
			w.identity.FailOnce("UsersByID", boom)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newFakeWorld(t, false)
			facts := &factsFake{open: w.nvda(), hasOpen: true, voted: 5, eligible: 8}
			deps := w.monthDeps(facts, nil, repliesFake{})
			arrange(w, &deps, facts)
			_, err := app.NewInvitePreviewQuery(deps).Run(t.Context(), "andres", w.cabal)
			if err == nil {
				t.Fatal("Run err = nil, want the read failure")
			}
		})
	}
}

func TestGetReferralCabal_aReturnBeyondInt32IsClampedNotWrapped(t *testing.T) {
	t.Parallel()
	w := newFakeWorld(t, false)
	for want, bps := range map[int32]int64{math.MaxInt32: math.MaxInt64, math.MinInt32: math.MinInt64} {
		out, err := adapters.HTTP{Cabals: w.monthQuery(&factsFake{}, &bps, repliesFake{})}.GetReferralCabal(
			t.Context(), api.GetReferralCabalRequestObject{Code: "andres", CabalId: w.cabal.UUID()})
		if err != nil {
			t.Fatal(err)
		}
		got := out.(api.GetReferralCabal200JSONResponse).Body.Return1mBps
		if got == nil || *got != want {
			t.Errorf("return_1m_bps for %d = %v, want %d", bps, got, want)
		}
	}
}

func seedOpenProposal(
	t *testing.T, s server, w previewWorld, proposer ids.UserID, created time.Time, ballots map[string]int,
) uuid.UUID {
	t.Helper()
	id := ids.Real{}.NewV7()
	if _, err := s.pool.Exec(t.Context(), `INSERT INTO proposals (id, cabal_id, proposer_id, kind, symbol, mint,
		usdc_micros, thesis, quote_out_amount, threshold, status, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'buy', 'AAPLx', $4, 400000000, $5, 1, 'majority', 'open', $6, $7, $7)`,
		id, w.cabal.UUID(), proposer.UUID(), marketfake.AAPLx().Mint.String(),
		"small add before earnings\nsecond line", created.Add(5*time.Hour), created); err != nil {
		t.Fatal(err)
	}
	for i := range ballots["eligible"] {
		voter := ids.Real{}.NewV7()
		if _, err := s.pool.Exec(t.Context(),
			`INSERT INTO proposal_voters (proposal_id, voter_id) VALUES ($1, $2)`, id, voter); err != nil {
			t.Fatal(err)
		}
		choice := ""
		switch {
		case i < ballots["yes"]:
			choice = "yes"
		case i < ballots["yes"]+ballots["no"]:
			choice = "no"
		default:
			continue
		}
		castBallot(t, s, id, voter, choice, created)
	}
	return id
}

func castBallot(t *testing.T, s server, proposal, voter uuid.UUID, choice string, at time.Time) {
	t.Helper()
	if _, err := s.pool.Exec(t.Context(),
		`INSERT INTO votes (proposal_id, voter_id, choice, cast_at) VALUES ($1, $2, $3, $4)`,
		proposal, voter, choice, at); err != nil {
		t.Fatal(err)
	}
}

func seedMonthFacts(t *testing.T, s server, w previewWorld, now time.Time) uuid.UUID {
	t.Helper()
	if _, err := s.pool.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		logo_url, issuer_tradable, company_key, first_seen_at, updated_at)
		VALUES ($1, 'AAPLx', $2, 8, 'xstocks', 'equity', 'Apple', 'https://img.example/aapl.png', true, 'apple',
		$3, $3)`,
		ids.Real{}.NewV7(), marketfake.AAPLx().Mint.String(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(t.Context(), `INSERT INTO leaderboard_entries (board, range, rank, subject_id,
		subject_name, subject_created_at, value_micros, pnl_micros, return_bps, prices_as_of, computed_at, join_mode,
		member_count) VALUES ('cabals', '1M', 1, $1, 'AC Capital', $2, 20000000000, -5, -410, $2, $2, 'request', 4)`,
		w.cabal.UUID(), now); err != nil {
		t.Fatal(err)
	}
	proposer := w.members[0].id
	older := map[string]int{"eligible": 2, "yes": 2}
	passed := seedOpenProposal(t, s, w, proposer, now.AddDate(0, 0, -30), older)
	tally := map[string]int{"eligible": 8, "yes": 4, "no": 1}
	open := seedOpenProposal(t, s, w, proposer, now.Add(-time.Hour), tally)
	if _, err := s.pool.Exec(t.Context(), `UPDATE proposals SET status = 'passed' WHERE id = $1`, passed); err != nil {
		t.Fatal(err)
	}
	seedReplies(t, s, open, w.members[1].id, 6, now)
	return open
}

func seedReplies(t *testing.T, s server, proposal uuid.UUID, author ids.UserID, n int, now time.Time) {
	t.Helper()
	object := ids.Real{}.NewV7()
	if _, err := s.pool.Exec(t.Context(), `INSERT INTO feed_objects (id, kind, ref_type, ref_id, title, payload,
		created_at, updated_at) VALUES ($1, 'proposal', 'proposals', $2, 'Buy AAPLx', '{}', $3, $3)`,
		object, proposal, now); err != nil {
		t.Fatal(err)
	}
	for range n {
		if _, err := s.pool.Exec(t.Context(), `INSERT INTO feed_comments
			(id, feed_object_id, author_id, body, created_at) VALUES ($1, $2, $3, 'in', $4)`, ids.Real{}.NewV7(), object, author.UUID(), now); err != nil {
			t.Fatal(err)
		}
	}
}

func readPreview(t *testing.T, s server, cabal ids.CabalID) (map[string]any, string) {
	t.Helper()
	rec := s.preview(t, "k7m4qx2p", cabal)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET preview = %d %s", rec.Code, rec.Body)
	}
	return previewBody(t, rec), rec.Body.String()
}

func TestInvitePreview_theMonthFactsComeFromGovernanceRankingAndSocial(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	w := seedPreviewWorld(t, s, 3)
	now := s.clock.Now().UTC()
	open := seedMonthFacts(t, s, w, now)

	body, raw := readPreview(t, s, w.cabal)
	want := map[string]any{
		"id": open.String(), "kind": "buy", "symbol": "AAPLx", "asset_name": "Apple",
		"asset_logo_url": "https://img.example/aapl.png", "usdc_micros": "400000000", "proposer_name": "Member 0",
		"title": "small add before earnings", "yes": json.Number("4"), "no": json.Number("1"),
		"voted": json.Number("5"), "eligible": json.Number("8"), "replies": json.Number("6"),
		"expires_at": now.Add(4 * time.Hour).Format(time.RFC3339),
	}
	if got := body["open_proposal"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("open_proposal = %v, want %v", got, want)
	}
	if body["return_1m_bps"] != json.Number("-410") || body["turnout_month_bps"] != json.Number("6250") {
		t.Fatalf("return_1m_bps and turnout_month_bps = %v and %v, want -410 and 6250 (5 of 8 this month)",
			body["return_1m_bps"], body["turnout_month_bps"])
	}
	for _, problem := range keyProblems("", body) {
		t.Error(problem)
	}
	if strings.Contains(raw, w.members[0].id.String()) {
		t.Errorf("body %s holds the proposer's id", raw)
	}
}

func TestInvitePreview_aVoteBetweenTwoReadsMovesTheTally(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	w := seedPreviewWorld(t, s, 3)
	open := seedMonthFacts(t, s, w, s.clock.Now().UTC())
	castBallot(t, s, open, ids.Real{}.NewV7(), "no", s.clock.Now())
	if _, err := s.pool.Exec(t.Context(),
		`INSERT INTO proposal_voters (proposal_id, voter_id) SELECT proposal_id, voter_id FROM votes
		 WHERE proposal_id = $1 ON CONFLICT DO NOTHING`, open); err != nil {
		t.Fatal(err)
	}
	body, _ := readPreview(t, s, w.cabal)
	moved, _ := body["open_proposal"].(map[string]any)
	if moved["no"] != json.Number("2") || moved["voted"] != json.Number("6") || moved["yes"] != json.Number("4") {
		t.Fatalf("open_proposal after a vote = %v, want 4 yes, 2 no, 6 voted", moved)
	}
}
