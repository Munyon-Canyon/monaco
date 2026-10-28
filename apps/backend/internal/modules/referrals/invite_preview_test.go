package referrals_test

import (
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/referralsapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
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
	if strings.HasSuffix(name, "id") && key != "cabal.id" {
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
	identity *fakes.Identity
	cabals   *fakes.Cabal
	pots     *fakes.Treasury
	query    *app.InvitePreviewQuery
}

func newFakeWorld(t *testing.T, deleted bool) fakeWorld {
	t.Helper()
	gen := testkit.NewIDs(7)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	w := fakeWorld{cabal: ids.CabalIDFrom(gen.NewV7()), inviter: ids.NewUserID(gen)}
	friend := ids.NewUserID(gen)
	card := func(id ids.UserID, handle string, gone bool) identity.UserCard {
		return identity.UserCard{
			ID: id, Handle: handle, AccountStatus: identity.AccountActive, FirstDepositAt: &now, Deleted: gone,
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
