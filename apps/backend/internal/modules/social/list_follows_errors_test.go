package social_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type usersFailingOn struct {
	app.Users
	call   atomic.Int32
	onNth  int32
	cancel context.CancelFunc
}

func (u *usersFailingOn) UsersByID(
	ctx context.Context, userIDs []ids.UserID,
) (map[ids.UserID]identity.UserCard, error) {
	if u.call.Add(1) == u.onNth {
		if u.cancel != nil {
			u.cancel()
			return u.Users.UsersByID(ctx, userIDs)
		}
		return nil, errs.New(errs.CodeInternal, "test")
	}
	return u.Users.UsersByID(ctx, userIDs)
}

func TestListFollows_QueryFailures(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.insertFollow(t, f.bob, f.alice)
	f.insertFollow(t, f.alice, f.bob)
	q := app.FollowsQuery{Viewer: f.alice, User: f.alice, Limit: app.FollowsPageDefault}
	lists := map[string]func(context.Context, sqlc.DBTX, app.Users, app.FollowsQuery) (app.FollowsPage, error){
		"followers": app.ListFollowers,
		"following": app.ListFollowing,
	}
	for name, list := range lists {
		cancelled, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := list(cancelled, f.pool, f.users, q); err == nil {
			t.Fatalf("%s: row read failure succeeded", name)
		}
		ctx, stop := context.WithCancel(t.Context())
		lookup := &usersFailingOn{Users: f.users, onNth: 2, cancel: stop}
		if _, err := list(ctx, f.pool, lookup, q); err == nil {
			t.Fatalf("%s: followed lookup failure succeeded", name)
		}
		stop()
		users := &usersFailingOn{Users: f.users, onNth: 2}
		if _, err := list(t.Context(), f.pool, users, q); err == nil {
			t.Fatalf("%s: card lookup failure succeeded", name)
		}
		bad := q
		bad.Limit = 0
		if _, err := list(t.Context(), f.pool, f.users, bad); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("%s: zero limit err = %v, want invalid input", name, err)
		}
	}
}

func TestListFollowers_FullPageAtBatchBoundary(t *testing.T) {
	t.Parallel()
	f := newFollowsFixture(t, app.FollowsPageMax+2)
	cards := make([]identity.UserCard, 0, 2+len(f.followers))
	cards = append(cards,
		identity.UserCard{ID: f.target, Handle: "target", AccountStatus: identity.AccountActive},
		identity.UserCard{ID: f.viewer, Handle: "viewer", AccountStatus: identity.AccountActive},
	)
	for i, follower := range f.followers {
		status := identity.AccountActive
		if i == len(f.followers)-1 {
			status = identity.AccountBanned
		}
		cards = append(cards, identity.UserCard{
			ID: follower, Handle: "u" + follower.String()[:8], AccountStatus: status,
		})
		f.insert(t, follower, f.target, f.clock.Now().Add(time.Duration(i)*time.Second))
	}
	f.users = fakes.NewIdentity(cards, nil)
	page, err := app.ListFollowers(t.Context(), f.pool, f.users,
		app.FollowsQuery{Viewer: f.viewer, User: f.target, Limit: app.FollowsPageMax})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != app.FollowsPageMax || page.Next == nil {
		t.Fatalf("items = %d, next = %v, want a full page with a cursor", len(page.Items), page.Next)
	}
	if page.Next.ID != page.Items[len(page.Items)-1].User.ID.UUID() {
		t.Fatalf("cursor %v does not point at the last item", page.Next)
	}
}

func TestListFollowingRoute_pagesAndFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.insertFollow(t, f.alice, f.bob)
	f.insertFollow(t, f.alice, f.deleted)
	ctx := asUser(t.Context(), f.alice)
	res, err := f.routes().GetUserFollowing(ctx, api.GetUserFollowingRequestObject{
		Id: f.alice.UUID(), Params: api.GetUserFollowingParams{Limit: ptr(1)},
	})
	page, ok := res.(api.GetUserFollowing200JSONResponse)
	if err != nil || !ok || len(page.Items) != 1 || !page.Items[0].FollowedByMe {
		t.Fatalf("res = %#v, err = %v", res, err)
	}
	for _, params := range []api.GetUserFollowingParams{{Cursor: ptr("bad")}, {Limit: ptr(0)}} {
		if _, err := f.routes().GetUserFollowing(ctx, api.GetUserFollowingRequestObject{
			Id: f.alice.UUID(), Params: params,
		}); err == nil {
			t.Fatalf("params %+v succeeded", params)
		}
	}
	if _, err := f.routes().GetUserFollowing(t.Context(), api.GetUserFollowingRequestObject{
		Id: f.alice.UUID(),
	}); err == nil {
		t.Fatal("anonymous request succeeded")
	}
	if _, err := f.routes().GetUserFollowers(ctx, api.GetUserFollowersRequestObject{
		Id: f.stranger.UUID(),
	}); err == nil {
		t.Fatal("unknown user succeeded")
	}
}

func TestListFollowersRoute_cursorRoundTrip(t *testing.T) {
	t.Parallel()
	f := newFollowsFixture(t, 2)
	for i, follower := range f.followers {
		f.insert(t, follower, f.target, f.clock.Now().Add(time.Duration(i)*time.Second))
	}
	routes := social.HTTPOf(social.New(
		module.Deps{Pool: f.pool, IDs: f.gen, Clock: f.clock}, social.WithUsers(f.users),
	))
	ctx := asUser(t.Context(), f.viewer)
	first, err := routes.GetUserFollowers(ctx, api.GetUserFollowersRequestObject{
		Id: f.target.UUID(), Params: api.GetUserFollowersParams{Limit: ptr(1)},
	})
	page, ok := first.(api.GetUserFollowers200JSONResponse)
	if err != nil || !ok || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatalf("first = %#v, err = %v", first, err)
	}
	second, err := routes.GetUserFollowers(ctx, api.GetUserFollowersRequestObject{
		Id: f.target.UUID(), Params: api.GetUserFollowersParams{Cursor: page.NextCursor, Limit: ptr(1)},
	})
	rest, ok := second.(api.GetUserFollowers200JSONResponse)
	if err != nil || !ok || len(rest.Items) != 1 || rest.NextCursor != nil {
		t.Fatalf("second = %#v, err = %v", second, err)
	}
	if rest.Items[0].UserId == page.Items[0].UserId {
		t.Fatal("the cursor repeated an item")
	}
}
