package social_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type followsFixture struct {
	pool      *pgxpool.Pool
	gen       *testkit.IDs
	clock     *testkit.Clock
	users     *fakes.Identity
	target    ids.UserID
	viewer    ids.UserID
	followers []ids.UserID
}

func newFollowsFixture(t *testing.T, count int) followsFixture {
	t.Helper()
	gen := testkit.NewIDs(587)
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Second))
	target, viewer := ids.NewUserID(gen), ids.NewUserID(gen)
	cards := make([]identity.UserCard, 0, 2+count)
	cards = append(cards,
		identity.UserCard{ID: target, Handle: "target", AccountStatus: identity.AccountActive},
		identity.UserCard{ID: viewer, Handle: "viewer", AccountStatus: identity.AccountActive},
	)
	followers := make([]ids.UserID, count)
	for i := range followers {
		followers[i] = ids.NewUserID(gen)
		cards = append(
			cards,
			identity.UserCard{
				ID:            followers[i],
				Handle:        "user" + followers[i].String()[:8],
				AccountStatus: identity.AccountActive,
			},
		)
	}
	return followsFixture{
		pool: testkit.DB(t), gen: gen, clock: clk, users: fakes.NewIdentity(cards, nil),
		target: target, viewer: viewer, followers: followers,
	}
}

func (f followsFixture) insert(t *testing.T, follower, followee ids.UserID, at time.Time) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO follows (id, follower_id, followee_id, created_at)
		VALUES ($1, $2, $3, $4)`, f.gen.NewV7(), follower.UUID(), followee.UUID(), at)
	if err != nil {
		t.Fatal(err)
	}
}

func (f followsFixture) list(t *testing.T, after *domain.Keyset) app.FollowsPage {
	t.Helper()
	page, err := app.ListFollowers(t.Context(), f.pool, f.users,
		app.FollowsQuery{Viewer: f.viewer, User: f.target, After: after, Limit: app.FollowsPageDefault})
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestListFollowers_KeysetPagination(t *testing.T) {
	t.Parallel()
	f := newFollowsFixture(t, 75)
	for i, follower := range f.followers {
		f.insert(t, follower, f.target, f.clock.Now().Add(time.Duration(i)*time.Second))
	}
	got := map[uuid.UUID]bool{}
	var after *domain.Keyset
	for {
		page := f.list(t, after)
		for _, item := range page.Items {
			got[item.User.ID.UUID()] = true
		}
		if page.Next == nil {
			break
		}
		cursor, err := domain.ParseKeyset(page.Next.Encode())
		if err != nil {
			t.Fatal(err)
		}
		after = &cursor
	}
	if len(got) != len(f.followers) {
		t.Fatalf("listed %d unique followers, want 75", len(got))
	}
}

func TestListFollowers_HidesBannedAndDeleted(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.insertFollow(t, f.bob, f.alice)
	f.insertFollow(t, f.banned, f.alice)
	f.insertFollow(t, f.deleted, f.alice)
	page, err := app.ListFollowers(t.Context(), f.pool, f.users,
		app.FollowsQuery{Viewer: f.alice, User: f.alice, Limit: app.FollowsPageDefault})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].User.ID != f.bob || page.Items[0].FollowedBy {
		t.Fatalf("page = %+v, want only bob", page)
	}
}

func TestListFollows_followingAndFailures(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.insertFollow(t, f.alice, f.bob)
	f.insertFollow(t, f.bob, f.alice)
	res, err := f.routes().
		GetUserFollowers(asUser(t.Context(), f.alice), api.GetUserFollowersRequestObject{Id: f.alice.UUID()})
	if err != nil || len(res.(api.GetUserFollowers200JSONResponse).Items) != 1 {
		t.Fatalf("route = %#v, err = %v", res, err)
	}
	page, err := app.ListFollowing(t.Context(), f.pool, f.users,
		app.FollowsQuery{Viewer: f.alice, User: f.alice, Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].User.ID != f.bob || !page.Items[0].FollowedBy {
		t.Fatalf("page = %+v, err = %v", page, err)
	}
	routes := f.routes()
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: f.alice.String()})
	if _, err := routes.GetUserFollowers(ctx, api.GetUserFollowersRequestObject{Id: f.stranger.UUID()}); err == nil {
		t.Fatal("unknown user succeeded")
	}
	f.users.Fail("UsersByID", errs.New(errs.CodeInternal, "test"))
	if _, err := app.ListFollowers(context.Background(), f.pool, f.users,
		app.FollowsQuery{User: f.alice, Limit: 1}); err == nil {
		t.Fatal("user lookup failure succeeded")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := app.ListFollowers(
		ctx,
		f.pool,
		fakes.NewIdentity([]identity.UserCard{{ID: f.alice, AccountStatus: identity.AccountActive}}, nil),
		app.FollowsQuery{User: f.alice, Limit: 1},
	); err == nil {
		t.Fatal("cancelled read succeeded")
	}
}

func TestListFollowsRoutes_errors(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := asUser(t.Context(), f.alice)
	for _, request := range []api.GetUserFollowersRequestObject{
		{Id: f.alice.UUID(), Params: api.GetUserFollowersParams{Cursor: ptr("bad")}},
		{Id: f.alice.UUID(), Params: api.GetUserFollowersParams{Limit: ptr(101)}},
	} {
		if _, err := f.routes().GetUserFollowers(ctx, request); err == nil {
			t.Fatal("invalid request succeeded")
		}
	}
	if _, err := f.routes().
		GetUserFollowing(t.Context(), api.GetUserFollowingRequestObject{Id: f.alice.UUID()}); err == nil {
		t.Fatal("anonymous request succeeded")
	}
}

func (f fixture) insertFollow(t *testing.T, follower, followee ids.UserID) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO follows (id, follower_id, followee_id, created_at)
		VALUES ($1, $2, $3, $4)`, f.gen.NewV7(), follower.UUID(), followee.UUID(), f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
}
