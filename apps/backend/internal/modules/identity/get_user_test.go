package identity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/identityapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type stubFollows struct{ countsErr, followedErr, blockedErr error }

func (s stubFollows) Counts(context.Context, ids.UserID) (int, int, error) { return 0, 0, s.countsErr }

func (s stubFollows) FollowedByMe(context.Context, ids.UserID, ids.UserID) (bool, error) {
	return false, s.followedErr
}

func (s stubFollows) FollowedAmong(context.Context, ids.UserID, []ids.UserID) (map[ids.UserID]bool, error) {
	return nil, s.followedErr
}

func (s stubFollows) BlockedByMe(context.Context, ids.UserID, ids.UserID) (bool, error) {
	return false, s.blockedErr
}

type stubCabals struct {
	memberships []ids.CabalID
	err         error
}

func (s stubCabals) CabalsOf(context.Context, ids.UserID) ([]ids.CabalID, error) {
	return s.memberships, s.err
}

func stubMemberships(n int) stubCabals {
	gen := testkit.NewIDs(900)
	out := make([]ids.CabalID, n)
	for i := range out {
		out[i] = ids.CabalIDFrom(gen.NewV7())
	}
	return stubCabals{memberships: out}
}

func (f portFixture) block(t *testing.T, blocker, blocked ids.UserID) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO user_blocks (id, blocker_id, blocked_id, created_at)
		VALUES ($1, $2, $3, $4)`, f.ids.NewV7(), blocker.UUID(), blocked.UUID(), f.now)
	portOK(t, err)
}

func (f portFixture) follow(t *testing.T, follower, followee ids.UserID) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO follows (id, follower_id, followee_id, created_at)
		VALUES ($1, $2, $3, $4)`, f.ids.NewV7(), follower.UUID(), followee.UUID(), f.now)
	portOK(t, err)
}

func TestGetUser_SocialFields(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	alice := f.seed(t, portSeed{handle: "alice", name: "Alice"})
	bob := f.seed(t, portSeed{handle: "bob", name: "Bob", photo: "https://img.example/bob.png"})
	carol := f.seed(t, portSeed{handle: "carol", name: "Carol"})
	f.follow(t, alice.ID, bob.ID)
	f.follow(t, carol.ID, bob.ID)
	f.follow(t, bob.ID, alice.ID)
	routes := adapters.HTTP{
		Cards: adapters.NewQueries(f.pool), Follows: social.New(module.Deps{Pool: f.pool}).FollowCounts(),
		Cabals: stubMemberships(3),
	}
	other := getUser(t, routes, alice.ID, bob.ID)
	if other.Handle != "bob" || other.DisplayName != "Bob" || other.PhotoUrl == nil {
		t.Fatalf("other profile = %#v", other)
	}
	if got := socialFields(other); got != (fields{2, 1, true, false}) {
		t.Fatalf("other social fields = %+v", got)
	}
	self := getUser(t, routes, alice.ID, alice.ID)
	if got := socialFields(self); got != (fields{1, 1, false, false}) || self.PhotoUrl != nil {
		t.Fatalf("self social fields = %+v, photo = %v", got, self.PhotoUrl)
	}
	f.block(t, alice.ID, bob.ID)
	if blocked := getUser(t, routes, alice.ID, bob.ID); !blocked.BlockedByMe {
		t.Fatalf("after the block, blocked_by_me = %v, want true", blocked.BlockedByMe)
	}
	if theirs := getUser(t, routes, bob.ID, alice.ID); theirs.BlockedByMe {
		t.Fatalf("for the blocked user, blocked_by_me = %v, want false", theirs.BlockedByMe)
	}
	if _, err := f.pool.Exec(t.Context(), `DELETE FROM user_blocks`); err != nil {
		t.Fatal(err)
	}
	if unblocked := getUser(t, routes, alice.ID, bob.ID); unblocked.BlockedByMe {
		t.Fatalf("after the unblock, blocked_by_me = %v, want false", unblocked.BlockedByMe)
	}
}

type fields struct {
	followers, following int
	followedByMe         bool
	blockedByMe          bool
}

func socialFields(p api.GetUser200JSONResponse) fields {
	return fields{p.FollowerCount, p.FollowingCount, p.FollowedByMe, p.BlockedByMe}
}

func getUser(t *testing.T, routes adapters.HTTP, viewer, id ids.UserID) api.GetUser200JSONResponse {
	t.Helper()
	res, err := routes.GetUser(asUser(t, viewer), api.GetUserRequestObject{Id: id.UUID()})
	profile, ok := res.(api.GetUser200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("GetUser(%s) = %#v, err = %v", id, res, err)
	}
	return profile
}

func TestGetUser_hidesBannedDeletedAndUnknownUsers(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	viewer := f.seed(t, portSeed{handle: "viewer"})
	banned := f.seed(t, portSeed{handle: "banned", status: "banned"})
	deleted := f.seed(t, portSeed{handle: "gone", softDeleted: true})
	routes := adapters.HTTP{Cards: adapters.NewQueries(f.pool), Follows: stubFollows{}, Cabals: stubCabals{}}
	for _, id := range []ids.UserID{banned.ID, deleted.ID, f.newID(t)} {
		if _, err := routes.GetUser(asUser(t, viewer.ID), api.GetUserRequestObject{Id: id.UUID()}); errs.CodeOf(
			err,
		) != errs.CodeUserNotFound {
			t.Fatalf("GetUser(%s) = %v, want user_not_found", id, err)
		}
	}
}

func TestGetUser_failures(t *testing.T) {
	t.Parallel()
	gen := testkit.NewIDs(587)
	viewer, target := ids.NewUserID(gen), ids.NewUserID(gen)
	cards := fakes.NewIdentity([]app.UserCard{{ID: target, Handle: "target"}}, nil)
	down := errs.New(errs.CodeInternal, "test")
	if _, err := (adapters.HTTP{}).GetUser(t.Context(), api.GetUserRequestObject{Id: target.UUID()}); errs.CodeOf(
		err,
	) != errs.CodeUnauthorized {
		t.Fatalf("anonymous GetUser = %v, want unauthorized", err)
	}
	for name, follows := range map[string]app.FollowCounts{
		"unwired": app.UnwiredFollowCounts{}, "counts": stubFollows{countsErr: down},
		"followed": stubFollows{followedErr: down}, "blocked": stubFollows{blockedErr: down},
	} {
		routes := adapters.HTTP{Cards: cards, Follows: follows, Cabals: stubCabals{}}
		if _, err := routes.GetUser(asUser(t, viewer), api.GetUserRequestObject{Id: target.UUID()}); err == nil {
			t.Fatalf("%s: GetUser succeeded", name)
		}
	}
	_, _, err := (app.UnwiredFollowCounts{}).Counts(t.Context(), target)
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("unwired counts = %v, want upstream_unavailable", err)
	}
	if _, err := (app.UnwiredFollowCounts{}).FollowedByMe(t.Context(), viewer, target); errs.CodeOf(
		err,
	) != errs.CodeUpstreamUnavailable {
		t.Fatalf("unwired followed = %v, want upstream_unavailable", err)
	}
	if _, err := (app.UnwiredFollowCounts{}).BlockedByMe(t.Context(), viewer, target); errs.CodeOf(
		err,
	) != errs.CodeUpstreamUnavailable {
		t.Fatalf("unwired blocked = %v, want upstream_unavailable", err)
	}
	for name, cabals := range map[string]app.CabalCounts{
		"unwired": app.UnwiredCabalCounts{}, "down": stubCabals{err: down},
	} {
		routes := adapters.HTTP{Cards: cards, Follows: stubFollows{}, Cabals: cabals}
		if _, err := routes.GetUser(asUser(t, viewer), api.GetUserRequestObject{Id: target.UUID()}); err == nil {
			t.Fatalf("cabals %s: GetUser succeeded", name)
		}
	}
	if _, err := (app.UnwiredCabalCounts{}).CabalsOf(t.Context(), target); errs.CodeOf(
		err,
	) != errs.CodeUpstreamUnavailable {
		t.Fatalf("unwired cabals = %v, want upstream_unavailable", err)
	}
	cards.Fail("UsersByID", down)
	routes := adapters.HTTP{Cards: cards, Follows: stubFollows{}, Cabals: stubCabals{}}
	if _, err := routes.GetUser(asUser(t, viewer), api.GetUserRequestObject{Id: target.UUID()}); err == nil {
		t.Fatal("GetUser succeeded with the user lookup down")
	}
}

func TestModule_wireTakesFollowCountsFromSocialInTheBuiltSet(t *testing.T) {
	t.Parallel()
	d := module.Deps{}
	id := identity.New(d)
	module.NewSet(id, social.New(d))
	if _, unwired := id.Follows().(app.UnwiredFollowCounts); unwired {
		t.Fatal("follow counts stayed unwired with social in the set")
	}
	alone := identity.New(d)
	module.NewSet(alone)
	if _, unwired := alone.Follows().(app.UnwiredFollowCounts); !unwired {
		t.Fatal("follow counts were wired without social in the set")
	}
	injected := stubFollows{}
	winner := identity.New(d, identity.WithFollowCounts(injected))
	module.NewSet(winner, social.New(d))
	if winner.Follows() != any(injected) {
		t.Fatalf("follows = %T, want the injected stub", winner.Follows())
	}
}

func TestGetUser_overHTTPServesAnotherMembersProfile(t *testing.T) {
	t.Parallel()
	f := newHTTPFixtureIn(t, config.EnvTest, config.EnvTest,
		identity.WithFollowCounts(stubFollows{}), identity.WithCabalCounts(stubCabals{}))
	viewer := f.seed(t, portSeed{handle: "viewer"})
	other := f.seed(t, portSeed{handle: "other", name: "Other"})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/users/"+other.ID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+f.verifier.Mint(viewer.ID.String(), f.now.Add(time.Hour)))
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	var got api.GetUser200JSONResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK || got.Handle != "other" {
		t.Fatalf("GET /v1/users/{id} = %d %s, want 200 for @other (err %v)", rec.Code, rec.Body, err)
	}
}

func TestGetUser_servesTheBioJoinedDateAndCabalCount(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	viewer := f.seed(t, portSeed{handle: "viewer"})
	bob := f.seed(t, portSeed{handle: "bob", name: "Bob"})
	bare := f.seed(t, portSeed{handle: "bare", name: "Bare"})
	if _, err := f.pool.Exec(t.Context(), `UPDATE users SET bio = $2 WHERE id = $1`,
		bob.ID.UUID(), "Building in public.\nNYC"); err != nil {
		t.Fatal(err)
	}
	routes := adapters.HTTP{
		Cards: adapters.NewQueries(f.pool), Follows: stubFollows{}, Cabals: stubMemberships(4),
	}
	got := getUser(t, routes, viewer.ID, bob.ID)
	if got.Bio == nil || *got.Bio != "Building in public.\nNYC" || got.CabalCount != 4 ||
		!got.CreatedAt.Equal(f.created()) || got.CreatedAt.Location() != time.UTC {
		t.Fatalf("profile = bio %v, cabals %d, created %s", got.Bio, got.CabalCount, got.CreatedAt)
	}
	routes.Cabals = stubCabals{}
	if none := getUser(t, routes, viewer.ID, bare.ID); none.Bio != nil || none.CabalCount != 0 {
		t.Fatalf("profile without a bio or cabals = bio %v, cabals %d", none.Bio, none.CabalCount)
	}
}

func TestModule_wireTakesCabalCountsFromCabalInTheBuiltSet(t *testing.T) {
	t.Parallel()
	d := module.Deps{}
	id := identity.New(d)
	module.NewSet(id, cabal.New(d))
	if _, unwired := id.Cabals().(app.UnwiredCabalCounts); unwired {
		t.Fatal("cabal counts stayed unwired with cabal in the set")
	}
	alone := identity.New(d)
	module.NewSet(alone)
	if _, unwired := alone.Cabals().(app.UnwiredCabalCounts); !unwired {
		t.Fatal("cabal counts were wired without cabal in the set")
	}
	injected := stubCabals{}
	winner := identity.New(d, identity.WithCabalCounts(injected))
	module.NewSet(winner, cabal.New(d))
	if _, ok := winner.Cabals().(stubCabals); !ok {
		t.Fatalf("cabals = %T, want the injected stub", winner.Cabals())
	}
}
