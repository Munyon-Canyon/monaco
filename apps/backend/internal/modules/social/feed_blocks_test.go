package social_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (f feedFixture) setBlock(t *testing.T, blocker, blocked ids.UserID, on bool) {
	t.Helper()
	query := `INSERT INTO user_blocks (id, blocker_id, blocked_id, created_at) VALUES ($1, $2, $3, now())`
	args := []any{f.gen.NewV7(), blocker.UUID(), blocked.UUID()}
	if !on {
		query = `DELETE FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2`
		args = args[1:]
	}
	if _, err := f.pool.Exec(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func TestFeedBlocks_hideTheBlockedUsersItemsInEitherDirection(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer, author, bystander := ids.NewUserID(f.gen), ids.NewUserID(f.gen), ids.NewUserID(f.gen)
	cabal := ids.CabalIDFrom(f.gen.NewV7())
	byAuthor := f.item(t, func(it *feed.Item) { it.CabalID, it.ActorID = cabal, author })
	byOther := f.item(t, func(it *feed.Item) { it.CabalID, it.ActorID = cabal, bystander })
	system := f.item(t, func(it *feed.Item) { it.CabalID, it.ActorID = cabal, ids.UserID{} })
	all := []uuid.UUID{byAuthor, byOther, system}
	rest := []uuid.UUID{byOther, system}
	for name, set := range map[string][2]ids.UserID{
		"viewer blocked the author": {viewer, author},
		"author blocked the viewer": {author, viewer},
	} {
		f.setBlock(t, set[0], set[1], true)
		for _, sort := range []feed.Sort{"", feed.SortTop} {
			got := f.list(t, app.FeedQuery{Viewer: viewer, Sort: sort, Now: f.clock.Now()}).Items
			if !sameSet(idsOf(got), rest) {
				t.Errorf("%s, sort %q: items = %v, want %v", name, sort, idsOf(got), rest)
			}
		}
		f.wantVisibleExactly(t, viewer, app.FeedFilter{}, all, rest)
		if got := f.list(t, app.FeedQuery{Viewer: bystander}).Items; !sameSet(idsOf(got), all) {
			t.Errorf("%s hid items from a bystander: %v", name, idsOf(got))
		}
		f.setBlock(t, set[0], set[1], false)
		if got := f.list(t, app.FeedQuery{Viewer: viewer}).Items; !sameSet(idsOf(got), all) {
			t.Errorf("unblocked, %s: items = %v, want %v", name, idsOf(got), all)
		}
		f.wantVisibleExactly(t, viewer, app.FeedFilter{}, all, all)
	}
}
