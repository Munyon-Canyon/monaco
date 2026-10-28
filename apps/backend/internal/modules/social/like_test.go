package social_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type likeFixture struct {
	commentRoutes
	like   *app.LikeFeedItemHandler
	unlike *app.UnlikeFeedItemHandler
}

func newLikeFixture(t *testing.T) likeFixture {
	t.Helper()
	f := likeFixture{commentRoutes: newCommentRoutes(t)}
	deps := app.LikeDeps{UoW: f.uow, Reads: f.pool, Clock: f.clock, Hints: f.hints}
	f.like, f.unlike = app.NewLikeFeedItemHandler(deps), app.NewUnlikeFeedItemHandler(deps)
	return f
}

func (f likeFixture) put(t *testing.T, item uuid.UUID, viewer ids.UserID) error {
	t.Helper()
	return f.like.Handle(asUser(t.Context(), viewer), app.LikeFeedItem{FeedObjectID: item, Viewer: viewer})
}

func (f likeFixture) remove(t *testing.T, item uuid.UUID, viewer ids.UserID) error {
	t.Helper()
	return f.unlike.Handle(asUser(t.Context(), viewer), app.UnlikeFeedItem{FeedObjectID: item, Viewer: viewer})
}

func (f likeFixture) rows(t *testing.T, item uuid.UUID) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM feed_reactions WHERE feed_object_id = $1 AND emoji = $2`, item, redHeart).
		Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f likeFixture) detail(t *testing.T, item uuid.UUID, viewer ids.UserID) api.FeedItem {
	t.Helper()
	res, err := f.routes.GetFeedItem(asUser(t.Context(), viewer), api.GetFeedItemRequestObject{Id: item})
	if err != nil {
		t.Fatal(err)
	}
	return res.(api.GetFeedItem200JSONResponse).Item
}

func (f likeFixture) listed(t *testing.T, item uuid.UUID, viewer ids.UserID, sort api.GetFeedParamsSort) api.FeedItem {
	t.Helper()
	res, err := f.routes.GetFeed(asUser(t.Context(), viewer), api.GetFeedRequestObject{
		Params: api.GetFeedParams{Sort: &sort},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range res.(api.GetFeed200JSONResponse).Items {
		if got.Id == item {
			return got
		}
	}
	t.Fatalf("item %s is not in the %s feed", item, sort)
	return api.FeedItem{}
}

func TestLikeFeedItem_likingTwiceLeavesOneRowAndUnlikingTwiceLeavesNone(t *testing.T) {
	t.Parallel()
	f := newLikeFixture(t)
	item, me := f.item(t, feed.KindTrade), f.cabal.Members[0].ID
	for range 2 {
		if err := f.put(t, item, me); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.rows(t, item); got != 1 {
		t.Fatalf("%d like rows after two likes, want 1", got)
	}
	if got := len(f.hints.seen()); got != 1 {
		t.Fatalf("%d feed hints after two likes, want 1", got)
	}
	for range 2 {
		if err := f.remove(t, item, me); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.rows(t, item); got != 0 {
		t.Fatalf("%d like rows after two unlikes, want 0", got)
	}
	if got := len(f.hints.seen()); got != 2 {
		t.Fatalf("%d feed hints after a like and an unlike, want 2", got)
	}
}

func TestLikeFeedItem_countAndLikedByMeArePerViewer(t *testing.T) {
	t.Parallel()
	f := newLikeFixture(t)
	item, a, b := f.item(t, feed.KindTrade), f.cabal.Members[0].ID, f.cabal.Members[1].ID
	if err := f.put(t, item, a); err != nil {
		t.Fatal(err)
	}
	if err := f.put(t, item, f.outsider); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		viewer ids.UserID
		liked  bool
	}{{a, true}, {f.outsider, true}, {b, false}} {
		for name, got := range map[string]api.FeedItem{
			"get": f.detail(t, item, tt.viewer), "new": f.listed(t, item, tt.viewer, api.New),
			"top": f.listed(t, item, tt.viewer, api.Top),
		} {
			if got.LikeCount != 2 || got.LikedByMe != tt.liked {
				t.Errorf("%s as %s: like_count %d, liked_by_me %v, want 2 and %v",
					name, tt.viewer, got.LikeCount, got.LikedByMe, tt.liked)
			}
		}
	}
	if err := f.remove(t, item, a); err != nil {
		t.Fatal(err)
	}
	if got := f.detail(t, item, a); got.LikeCount != 1 || got.LikedByMe {
		t.Fatalf("after unliking: like_count %d, liked_by_me %v, want 1 and false", got.LikeCount, got.LikedByMe)
	}
}

func TestLikeFeedItem_throughTheRoutes(t *testing.T) {
	t.Parallel()
	f := newLikeFixture(t)
	item, me := f.item(t, feed.KindTrade), f.cabal.Members[0].ID
	ctx := asUser(t.Context(), me)
	put, err := f.routes.PutFeedLike(ctx, api.PutFeedLikeRequestObject{Id: item})
	if err != nil || put != (api.PutFeedLike204Response{}) {
		t.Fatalf("put = %v, %v, want 204", put, err)
	}
	if got := f.detail(t, item, me); got.LikeCount != 1 || !got.LikedByMe {
		t.Fatalf("after put: like_count %d, liked_by_me %v, want 1 and true", got.LikeCount, got.LikedByMe)
	}
	del, err := f.routes.DeleteFeedLike(ctx, api.DeleteFeedLikeRequestObject{Id: item})
	if err != nil || del != (api.DeleteFeedLike204Response{}) {
		t.Fatalf("delete = %v, %v, want 204", del, err)
	}
	if got := f.detail(t, item, me); got.LikeCount != 0 || got.LikedByMe {
		t.Fatalf("after delete: like_count %d, liked_by_me %v, want 0 and false", got.LikeCount, got.LikedByMe)
	}
	anon := t.Context()
	if _, err := f.routes.PutFeedLike(anon, api.PutFeedLikeRequestObject{Id: item}); err == nil {
		t.Fatal("put without a caller = nil error")
	}
	if _, err := f.routes.DeleteFeedLike(anon, api.DeleteFeedLikeRequestObject{Id: item}); err == nil {
		t.Fatal("delete without a caller = nil error")
	}
}

func TestLikeFeedItem_refusesAnItemTheViewerCannotSee(t *testing.T) {
	t.Parallel()
	f := newLikeFixture(t)
	me := f.cabal.Members[0].ID
	hidden, unknown := f.item(t, feed.KindTrade), f.gen.NewV7()
	if _, err := f.pool.Exec(t.Context(),
		`INSERT INTO feed_cabals (cabal_id, name, updated_at, hidden_at) VALUES ($1, 'Alpha Cabal', now(), now())
		 ON CONFLICT (cabal_id) DO UPDATE SET hidden_at = now()`, f.cabal.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	for name, item := range map[string]uuid.UUID{"hidden": hidden, "unknown": unknown} {
		wantCode(t, f.put(t, item, me), errs.CodeFeedItemNotFound)
		wantCode(t, f.remove(t, item, me), errs.CodeFeedItemNotFound)
		if got := f.rows(t, item); got != 0 {
			t.Errorf("%s item has %d like rows after refusals, want 0", name, got)
		}
	}
	if got := len(f.hints.seen()); got != 0 {
		t.Fatalf("%d feed hints after refusals, want 0", got)
	}
}

func TestLikeFeedItem_failsWithInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	f := newLikeFixture(t)
	item, me := f.item(t, feed.KindTrade), f.cabal.Members[0].ID
	if err := f.put(t, item, me); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE FUNCTION refuse_likes() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'no'; END $$`,
		`CREATE TRIGGER refuse BEFORE INSERT OR DELETE ON feed_reactions FOR EACH ROW EXECUTE FUNCTION refuse_likes()`,
	} {
		if _, err := f.pool.Exec(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
	wantCode(t, f.remove(t, item, me), errs.CodeInternal)
	wantCode(t, f.put(t, item, f.cabal.Members[1].ID), errs.CodeInternal)
}
