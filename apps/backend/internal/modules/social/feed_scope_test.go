package social_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (f feedFixture) follow(t *testing.T, follower, followee ids.UserID) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(),
		`INSERT INTO follows (id, follower_id, followee_id, source, created_at) VALUES ($1, $2, $3, 'profile', now())`,
		f.gen.NewV7(), follower.UUID(), followee.UUID())
	if err != nil {
		t.Fatal(err)
	}
}

type scopeWorld struct {
	f                        feedFixture
	viewer, friend, stranger ids.UserID
	cabalA, cabalB, cabalC   ids.CabalID
	aByStranger, aByFriend   uuid.UUID
	bByFriend, bByStranger   uuid.UUID
	cByStranger, move        uuid.UUID
	all                      []uuid.UUID
}

func newScopeWorld(t *testing.T) scopeWorld {
	t.Helper()
	f := newFeedFixture(t)
	w := scopeWorld{
		f: f, viewer: ids.NewUserID(f.gen), friend: ids.NewUserID(f.gen), stranger: ids.NewUserID(f.gen),
		cabalA: ids.CabalIDFrom(f.gen.NewV7()), cabalB: ids.CabalIDFrom(f.gen.NewV7()),
		cabalC: ids.CabalIDFrom(f.gen.NewV7()),
	}
	post := func(cabal ids.CabalID, actor ids.UserID) uuid.UUID {
		f.clock.Advance(time.Second)
		return f.byActor(t, actor, cabal, feed.KindProposal)
	}
	w.aByStranger, w.aByFriend = post(w.cabalA, w.stranger), post(w.cabalA, w.friend)
	w.bByFriend, w.bByStranger = post(w.cabalB, w.friend), post(w.cabalB, w.stranger)
	w.cByStranger = post(w.cabalC, w.stranger)
	f.clock.Advance(time.Second)
	w.move = f.item(t, func(it *feed.Item) { it.Kind = feed.KindPriceMove })
	w.all = []uuid.UUID{
		w.aByStranger, w.aByFriend, w.bByFriend, w.bByStranger, w.cByStranger, w.move,
	}
	f.membership(t, w.cabalA, w.viewer, true)
	f.follow(t, w.viewer, w.friend)
	return w
}

func (w scopeWorld) wantScope(t *testing.T, step string, scope feed.Scope, want ...uuid.UUID) {
	t.Helper()
	filter := app.FeedFilter{Scope: scope}
	for _, sort := range []feed.Sort{feed.SortNew, feed.SortTop} {
		q := app.FeedQuery{Viewer: w.viewer, Filter: filter, Sort: sort, Now: w.f.clock.Now()}
		if got := idsOf(w.f.list(t, q).Items); !sameSet(got, want) {
			t.Errorf("%s: scope %s sort %v lists %v, want %v", step, scope, sort, got, want)
		}
	}
	w.f.wantVisibleExactly(t, w.viewer, filter, w.all, want)
}

func TestFeedQuery_ScopeNetworkAndOthers(t *testing.T) {
	t.Parallel()
	w := newScopeWorld(t)
	w.wantScope(t, "start", feed.ScopeNetwork, w.aByStranger, w.aByFriend, w.bByFriend)
	w.wantScope(t, "start", feed.ScopeOthers, w.bByFriend, w.bByStranger, w.cByStranger)
	w.wantScope(t, "start", feed.ScopeMine, w.aByStranger, w.aByFriend)
	w.wantScope(t, "start", feed.ScopeFollowing, w.aByFriend, w.bByFriend)
	w.wantScope(t, "start", feed.ScopeAll, w.all...)
}

func TestFeedQuery_ScopeNetworkListsAnItemOfBothKindsOnceNewestFirst(t *testing.T) {
	t.Parallel()
	w := newScopeWorld(t)
	q := app.FeedQuery{Viewer: w.viewer, Filter: app.FeedFilter{Scope: feed.ScopeNetwork}, Limit: 2}
	first := w.f.list(t, q)
	want := []uuid.UUID{w.bByFriend, w.aByFriend}
	if got := idsOf(first.Items); !slices.Equal(got, want) || first.Next == nil {
		t.Fatalf("first page = %v, next %v, want %v and a cursor", got, first.Next, want)
	}
	q.After = first.Next
	second := w.f.list(t, q)
	if got, want := idsOf(second.Items), []uuid.UUID{w.aByStranger}; !slices.Equal(got, want) || second.Next != nil {
		t.Fatalf("second page = %v, next %v, want %v and no cursor", got, second.Next, want)
	}
}

func TestFeedQuery_LeavingACabalMovesItsItemsFromNetworkToOthers(t *testing.T) {
	t.Parallel()
	w := newScopeWorld(t)
	w.f.membership(t, w.cabalA, w.viewer, false)
	w.wantScope(t, "after leaving", feed.ScopeNetwork, w.aByFriend, w.bByFriend)
	w.wantScope(t, "after leaving", feed.ScopeOthers,
		w.aByStranger, w.aByFriend, w.bByFriend, w.bByStranger, w.cByStranger)
}

func TestFeedQuery_MutedCabalLeavesNetworkAndBannedCabalLeavesOthers(t *testing.T) {
	t.Parallel()
	w := newScopeWorld(t)
	w.f.setMute(t, w.viewer, "cabal", w.cabalA.String(), true)
	w.wantScope(t, "muted A", feed.ScopeNetwork, w.bByFriend)
	w.f.setMute(t, w.viewer, "cabal", w.cabalA.String(), false)
	if err := w.f.ban(t, w.cabalC); err != nil {
		t.Fatal(err)
	}
	w.wantScope(t, "banned C", feed.ScopeOthers, w.bByFriend, w.bByStranger)
	w.wantScope(t, "banned C", feed.ScopeNetwork, w.aByStranger, w.aByFriend, w.bByFriend)
}

func TestFeedQuery_UnfollowingDropsTheFriendsItemsOutsideMyCabals(t *testing.T) {
	t.Parallel()
	w := newScopeWorld(t)
	if _, err := w.f.pool.Exec(t.Context(), `UPDATE follows SET deleted_at = now() WHERE follower_id = $1`,
		w.viewer.UUID()); err != nil {
		t.Fatal(err)
	}
	w.wantScope(t, "after unfollowing", feed.ScopeNetwork, w.aByStranger, w.aByFriend)
}

func TestFeedQuery_StatusOpenFindsAnOpenProposalBehindNewerClosedOnes(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer, actor, cabal := ids.NewUserID(f.gen), ids.NewUserID(f.gen), ids.CabalIDFrom(f.gen.NewV7())
	f.membership(t, cabal, viewer, true)
	post := func(status string) uuid.UUID {
		f.clock.Advance(time.Second)
		return f.item(t, func(it *feed.Item) {
			it.Kind, it.CabalID, it.ActorID, it.Status = feed.KindProposal, cabal, actor, status
		})
	}
	open := post("open")
	closed := make([]uuid.UUID, 0, 6)
	for range 6 {
		closed = append(closed, post("passed"))
	}
	filter := app.FeedFilter{Scope: feed.ScopeMine, Kinds: []feed.Kind{feed.KindProposal}, Status: "open"}
	page := f.list(t, app.FeedQuery{Viewer: viewer, Filter: filter, Limit: 5})
	if got := idsOf(page.Items); !slices.Equal(got, []uuid.UUID{open}) || page.Next != nil {
		t.Fatalf("open = %v, next %v, want only %s and no cursor", got, page.Next, open)
	}
	filter.Status = ""
	got := idsOf(f.list(t, app.FeedQuery{Viewer: viewer, Filter: filter, Limit: 5}).Items)
	if len(got) != 5 || slices.Contains(got, open) {
		t.Fatalf("without the filter the five newest are closed ones, got %v of %v", got, closed)
	}
	top := app.FeedQuery{Viewer: viewer, Sort: feed.SortTop, Now: f.clock.Now(), Filter: app.FeedFilter{Status: "open"}}
	if got := idsOf(f.list(t, top).Items); !slices.Equal(got, []uuid.UUID{open}) {
		t.Fatalf("top with status open = %v, want %s", got, open)
	}
}

func TestParseScope_NetworkAndOthers(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]feed.Scope{"network": feed.ScopeNetwork, "others": feed.ScopeOthers} {
		if got, err := feed.ParseScope(raw); err != nil || got != want {
			t.Errorf("ParseScope(%q) = %q, %v, want %q", raw, got, err, want)
		}
	}
	if _, err := feed.ParseScope("Network"); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Errorf("ParseScope(Network) err = %v, want invalid_input", err)
	}
}
