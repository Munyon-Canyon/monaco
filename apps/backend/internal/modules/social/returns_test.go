package social_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type fakeReturns struct {
	mu     sync.Mutex
	rows   map[ids.UserID]rankingport.PersonReturn
	ranges []rankingport.Range
	asked  [][]ids.UserID
}

func (f *fakeReturns) PeopleReturns(
	_ context.Context, rng rankingport.Range, users []ids.UserID,
) (map[ids.UserID]rankingport.PersonReturn, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ranges, f.asked = append(f.ranges, rng), append(f.asked, users)
	return f.rows, nil
}

type countingUsers struct {
	*fakes.Identity
	mu    sync.Mutex
	calls int
}

func (c *countingUsers) UsersByID(ctx context.Context, who []ids.UserID) (map[ids.UserID]port.UserCard, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.Identity.UsersByID(ctx, who)
}

func bps(v int64) rankingport.PersonReturn {
	return rankingport.PersonReturn{ReturnBps: &v, AsOf: time.Unix(1, 0).UTC()}
}

func TestGetFeed_showsTheAuthorsReturnUnlessTheyHideIt(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer := ids.NewUserID(f.gen)
	shares, hides, unranked, unrankedRow := ids.NewUserID(f.gen), ids.NewUserID(f.gen),
		ids.NewUserID(f.gen), ids.NewUserID(f.gen)
	byActor := map[ids.UserID]uuid.UUID{}
	for _, actor := range []ids.UserID{shares, hides, unranked, unrankedRow, {}} {
		byActor[actor] = f.item(t, func(it *feed.Item) { it.RefID, it.ActorID = f.gen.NewV7(), actor })
		f.clock.Advance(time.Millisecond)
	}
	users := &countingUsers{Identity: fakes.NewIdentity([]identity.UserCard{
		{ID: shares, Handle: "a", SharePerformance: true},
		{ID: hides, Handle: "b"},
		{ID: unranked, Handle: "c", SharePerformance: true},
		{ID: unrankedRow, Handle: "d", SharePerformance: true},
	}, nil)}
	returns := &fakeReturns{rows: map[ids.UserID]rankingport.PersonReturn{
		shares: bps(820), hides: bps(-310), unrankedRow: {AsOf: time.Unix(1, 0).UTC()},
	}}
	deps := module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock}
	routes := social.HTTPOf(social.New(deps, social.WithUsers(users), social.WithReturns(returns)))
	res, err := routes.GetFeed(asUser(t.Context(), viewer), api.GetFeedRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	page := api.FeedPage(res.(api.GetFeed200JSONResponse))
	if len(page.Items) != 5 {
		t.Fatalf("items = %d, want 5", len(page.Items))
	}
	for actor, want := range map[ids.UserID]struct {
		bps     *int64
		private bool
	}{
		shares: {ptr(int64(820)), false}, hides: {nil, true}, unranked: {nil, false},
		unrankedRow: {nil, false}, {}: {nil, false},
	} {
		got := itemOf(t, page, byActor[actor])
		if (got.Return1mBps == nil) != (want.bps == nil) ||
			(want.bps != nil && *got.Return1mBps != *want.bps) || got.ReturnPrivate != want.private {
			t.Errorf("actor %s: return_1m_bps = %v, return_private = %v, want %v, %v",
				actor, got.Return1mBps, got.ReturnPrivate, want.bps, want.private)
		}
	}
	if users.calls != 1 || len(returns.asked) != 1 || returns.ranges[0] != rankingport.Range1M {
		t.Fatalf("UsersByID calls = %d, PeopleReturns calls = %d (%v), want one each for 1M",
			users.calls, len(returns.asked), returns.ranges)
	}
}

func TestGetFeedItem_showsTheAuthorsReturn(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer, author := ids.NewUserID(f.gen), ids.NewUserID(f.gen)
	id := f.item(t, func(it *feed.Item) { it.ActorID = author })
	users := &countingUsers{Identity: fakes.NewIdentity([]identity.UserCard{
		{ID: author, Handle: "a", SharePerformance: true},
	}, nil)}
	returns := &fakeReturns{rows: map[ids.UserID]rankingport.PersonReturn{author: bps(-45)}}
	deps := module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock}
	routes := social.HTTPOf(social.New(deps, social.WithUsers(users), social.WithReturns(returns)))
	res, err := routes.GetFeedItem(asUser(t.Context(), viewer), api.GetFeedItemRequestObject{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	got := api.FeedItemDetail(res.(api.GetFeedItem200JSONResponse)).Item
	if got.Return1mBps == nil || *got.Return1mBps != -45 || got.ReturnPrivate {
		t.Fatalf("item = %+v, want return -45 and not private", got)
	}
	assertOneLookup(t, users.calls, len(returns.asked))
}

func TestGetFeedComments_showsEachAuthorsReturnWithOneLookupPerPage(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item := f.item(t, feed.KindTrade)
	kai, gone := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	top := f.say(t, item, kai, "top", nil)
	f.say(t, item, f.outsider, "hidden", &top.Id)
	f.say(t, item, gone, "deleted", &top.Id)
	users := &countingUsers{Identity: fakes.NewIdentity([]identity.UserCard{
		{ID: kai, Handle: "kai", DisplayName: "Kai", SharePerformance: true},
		{ID: gone, Handle: "gone", Deleted: true, SharePerformance: true},
		{ID: f.outsider, Handle: "mia", DisplayName: "Mia"},
	}, nil)}
	returns := &fakeReturns{rows: map[ids.UserID]rankingport.PersonReturn{
		kai: bps(1250), f.outsider: bps(900), gone: bps(5),
	}}
	deps := module.Deps{Pool: f.pool, UoW: f.uow, IDs: f.gen, Clock: f.clock}
	routes := social.HTTPOf(social.New(deps, social.WithUsers(users), social.WithReturns(returns)))
	res, err := routes.GetFeedComments(asUser(t.Context(), kai), api.GetFeedCommentsRequestObject{Id: item})
	if err != nil {
		t.Fatal(err)
	}
	page := api.CommentPage(res.(api.GetFeedComments200JSONResponse))
	if len(page.Items) != 1 || len(page.Items[0].Replies) != 2 {
		t.Fatalf("page = %+v, want one thread with two replies", page)
	}
	if a := page.Items[0].Comment.Author; a.Return1mBps == nil || *a.Return1mBps != 1250 || a.ReturnPrivate {
		t.Errorf("sharing author = %+v, want 1250 and not private", a)
	}
	if a := page.Items[0].Replies[0].Author; a.Return1mBps != nil || !a.ReturnPrivate {
		t.Errorf("hiding author = %+v, want null and private", a)
	}
	if a := page.Items[0].Replies[1].Author; a.Return1mBps != nil || a.ReturnPrivate {
		t.Errorf("deleted author = %+v, want null and not private", a)
	}
	assertOneLookup(t, users.calls, len(returns.asked))
	assertOneLookup(t, users.calls, len(returns.asked))
}

func itemOf(t *testing.T, page api.FeedPage, id uuid.UUID) api.FeedItem {
	t.Helper()
	for _, item := range page.Items {
		if item.Id == id {
			return item
		}
	}
	t.Fatalf("item %s missing from %+v", id, page)
	return api.FeedItem{}
}

func assertOneLookup(t *testing.T, users, returns int) {
	t.Helper()
	if users != 1 || returns != 1 {
		t.Fatalf("UsersByID calls = %d, PeopleReturns calls = %d, want one each", users, returns)
	}
}

type brokenReturns struct{}

func (brokenReturns) PeopleReturns(
	context.Context, rankingport.Range, []ids.UserID,
) (map[ids.UserID]rankingport.PersonReturn, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "test")
}

func TestGetFeed_failsWhenTheReturnsLookupFails(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	f.item(t, func(it *feed.Item) { it.ActorID = ids.NewUserID(f.gen) })
	deps := module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock}
	routes := social.HTTPOf(social.New(deps,
		social.WithUsers(fakes.NewIdentity(nil, nil)), social.WithReturns(brokenReturns{})))
	_, err := routes.GetFeed(asUser(t.Context(), ids.NewUserID(f.gen)), api.GetFeedRequestObject{})
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("GetFeed = %v, want upstream_unavailable", err)
	}
}
