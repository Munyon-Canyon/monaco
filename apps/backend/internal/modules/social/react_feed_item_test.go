package social_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	baseapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type reactFixture struct {
	likeFixture
	react   *app.ReactFeedItemHandler
	unreact *app.UnreactFeedItemHandler
}

func newReactFixture(t *testing.T) reactFixture {
	t.Helper()
	f := reactFixture{likeFixture: newLikeFixture(t)}
	deps := app.ReactionDeps{UoW: f.uow, Reads: f.pool, Members: membersOn(f.pool), Clock: f.clock, Hints: f.hints}
	f.react, f.unreact = app.NewReactFeedItemHandler(deps), app.NewUnreactFeedItemHandler(deps)
	return f
}

func (f reactFixture) add(t *testing.T, item uuid.UUID, viewer ids.UserID, emoji string) (app.ReactionSummary, error) {
	t.Helper()
	cmd := app.ReactFeedItem{FeedObjectID: item, Viewer: viewer, Emoji: emoji}
	return f.react.Handle(asUser(t.Context(), viewer), cmd)
}

func (f reactFixture) drop(t *testing.T, item uuid.UUID, viewer ids.UserID, emoji string) (app.ReactionSummary, error) {
	t.Helper()
	cmd := app.UnreactFeedItem{FeedObjectID: item, Viewer: viewer, Emoji: emoji}
	return f.unreact.Handle(asUser(t.Context(), viewer), cmd)
}

func (f reactFixture) held(t *testing.T, item uuid.UUID, user ids.UserID) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM feed_reactions WHERE feed_object_id = $1 AND user_id = $2`, item, user.UUID()).
		Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f reactFixture) eventsOf(t *testing.T, typ events.Type) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type = $1`, string(typ)).
		Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func wantSummary(t *testing.T, got app.ReactionSummary, total int32, want ...app.Reaction) {
	t.Helper()
	if got.Total != total || len(got.Reactions) != len(want) {
		t.Fatalf("summary = %+v, want %d reactions %+v", got, total, want)
	}
	for _, w := range want {
		if !slices.Contains(got.Reactions, w) {
			t.Fatalf("summary = %+v, want %d reactions %+v", got, total, want)
		}
	}
}

func TestReactFeedItem_addingTwiceKeepsOneRowAndOneEvent(t *testing.T) {
	t.Parallel()
	f := newReactFixture(t)
	item, me := f.item(t, feed.KindTrade), f.cabal.Members[0].ID
	for range 2 {
		got, err := f.add(t, item, me, "🎉")
		if err != nil {
			t.Fatal(err)
		}
		wantSummary(t, got, 1, app.Reaction{Emoji: "🎉", Count: 1, Mine: true})
	}
	if got := f.held(t, item, me); got != 1 {
		t.Fatalf("%d rows after two identical reactions, want 1", got)
	}
	if got := f.eventsOf(t, events.TypeReactionAdded); got != 1 {
		t.Fatalf("%d reaction.added events after two identical reactions, want 1", got)
	}
	if got := len(f.hints.seen()); got != 1 {
		t.Fatalf("%d feed hints after two identical reactions, want 1", got)
	}
}

func TestReactFeedItem_aSecondEmojiAddsASecondRow(t *testing.T) {
	t.Parallel()
	f := newReactFixture(t)
	item, me, other := f.item(t, feed.KindTrade), f.cabal.Members[0].ID, f.cabal.Members[1].ID
	if _, err := f.add(t, item, other, "👀"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.add(t, item, me, "🎉"); err != nil {
		t.Fatal(err)
	}
	got, err := f.add(t, item, me, "👀")
	if err != nil {
		t.Fatal(err)
	}
	wantSummary(t, got, 3,
		app.Reaction{Emoji: "👀", Count: 2, Mine: true}, app.Reaction{Emoji: "🎉", Count: 1, Mine: true})
	if got := f.held(t, item, me); got != 2 {
		t.Fatalf("%d rows after two emoji, want 2", got)
	}
	if got := f.eventsOf(t, events.TypeReactionAdded); got != 3 {
		t.Fatalf("%d reaction.added events, want 3", got)
	}
}

func TestReactFeedItem_refusesTheEleventhEmojiButNotARepeat(t *testing.T) {
	t.Parallel()
	f := newReactFixture(t)
	item, me := f.item(t, feed.KindTrade), f.cabal.Members[0].ID
	tenth := []string{"😀", "😂", "😍", "😎", "😭", "😡", "🤔", "👍", "👀", "🎉"}
	for _, emoji := range tenth {
		if _, err := f.add(t, item, me, emoji); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.add(t, item, me, "🎉"); err != nil {
		t.Fatalf("repeating a held emoji at the cap: %v", err)
	}
	_, err := f.add(t, item, me, "🔥")
	wantCode(t, err, errs.CodeInvalidInput)
	var got *errs.Error
	if !errors.As(err, &got) || len(got.Attrs) != 1 || got.Attrs[0].Key != "reason" ||
		got.Attrs[0].Value.String() != "reaction_limit" {
		t.Fatalf("err attrs = %v, want reason=reaction_limit", err)
	}
	if got := f.held(t, item, me); got != 10 {
		t.Fatalf("%d rows after the refused 11th, want 10", got)
	}
	if got := f.eventsOf(t, events.TypeReactionAdded); got != 10 {
		t.Fatalf("%d reaction.added events after the refused 11th, want 10", got)
	}
	if _, err := f.add(t, item, f.cabal.Members[1].ID, "🔥"); err != nil {
		t.Fatalf("another user's first emoji: %v", err)
	}
}

func TestReactFeedItem_refusesWhatIsNotOneEmoji(t *testing.T) {
	t.Parallel()
	f := newReactFixture(t)
	item, me := f.item(t, feed.KindTrade), f.cabal.Members[0].ID
	for _, emoji := range []string{"", "a", "🎉🎉", "🎉 ", "👨‍👩‍👧‍👦🏽🏽🏽"} {
		_, err := f.add(t, item, me, emoji)
		wantCode(t, err, errs.CodeInvalidInput)
		_, err = f.drop(t, item, me, emoji)
		wantCode(t, err, errs.CodeInvalidInput)
	}
	if got := f.eventsOf(t, events.TypeReactionAdded); got != 0 {
		t.Fatalf("%d reaction.added events after refusals, want 0", got)
	}
}

func TestUnreactFeedItem_removesOnlyTheCallersEmojiAndAppendsAnEventOnlyWhenARowWent(t *testing.T) {
	t.Parallel()
	f := newReactFixture(t)
	item, me, other := f.item(t, feed.KindTrade), f.cabal.Members[0].ID, f.cabal.Members[1].ID
	for _, r := range []struct {
		user  ids.UserID
		emoji string
	}{{me, "🎉"}, {me, "👀"}, {other, "🎉"}} {
		if _, err := f.add(t, item, r.user, r.emoji); err != nil {
			t.Fatal(err)
		}
	}
	hints := len(f.hints.seen())
	got, err := f.drop(t, item, me, "🎉")
	if err != nil {
		t.Fatal(err)
	}
	wantSummary(t, got, 2, app.Reaction{Emoji: "👀", Count: 1, Mine: true}, app.Reaction{Emoji: "🎉", Count: 1})
	if held := f.held(t, item, me); held != 1 {
		t.Fatalf("%d rows left for the caller, want 1", held)
	}
	if held := f.held(t, item, other); held != 1 {
		t.Fatalf("%d rows left for the other user, want 1", held)
	}
	if _, err := f.drop(t, item, me, "🎉"); err != nil {
		t.Fatal(err)
	}
	if got := f.eventsOf(t, events.TypeReactionRemoved); got != 1 {
		t.Fatalf("%d reaction.removed events after two removals of one row, want 1", got)
	}
	if got := len(f.hints.seen()) - hints; got != 1 {
		t.Fatalf("%d feed hints after two removals of one row, want 1", got)
	}
}

func TestReactFeedItem_refusesAnItemTheViewerCannotSee(t *testing.T) {
	t.Parallel()
	f := newReactFixture(t)
	me := f.cabal.Members[0].ID
	_, err := f.add(t, f.gen.NewV7(), me, "🎉")
	wantCode(t, err, errs.CodeFeedItemNotFound)
	_, err = f.drop(t, f.gen.NewV7(), me, "🎉")
	wantCode(t, err, errs.CodeFeedItemNotFound)
}

func TestReactFeedItem_theEventNamesTheItemsActorAndCabal(t *testing.T) {
	t.Parallel()
	f := newReactFixture(t)
	item, me := f.item(t, feed.KindTrade), f.cabal.Members[0].ID
	if _, err := f.add(t, item, me, "🎉"); err != nil {
		t.Fatal(err)
	}
	var cabal uuid.UUID
	var emoji string
	var comment *string
	if err := f.pool.QueryRow(t.Context(),
		`SELECT (payload->>'cabal_id')::uuid, payload->>'emoji', payload->>'comment_id' FROM events WHERE type = $1`,
		string(events.TypeReactionAdded)).Scan(&cabal, &emoji, &comment); err != nil {
		t.Fatal(err)
	}
	if cabal != f.cabal.ID.UUID() || emoji != "🎉" || comment != nil {
		t.Fatalf("event cabal %s emoji %q comment %v, want %s, 🎉 and none", cabal, emoji, comment, f.cabal.ID.UUID())
	}
}

func TestLikeRoutes_writeARedHeartReactionAndNoLike(t *testing.T) {
	t.Parallel()
	f := newReactFixture(t)
	item, me := f.item(t, feed.KindTrade), f.cabal.Members[0].ID
	ctx := asUser(t.Context(), me)
	if _, err := f.routes.PutFeedLike(ctx, api.PutFeedLikeRequestObject{Id: item}); err != nil {
		t.Fatal(err)
	}
	var hearts, likes int
	if err := f.pool.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM feed_reactions WHERE feed_object_id = $1 AND emoji = $2),
		        (SELECT count(*) FROM feed_likes WHERE feed_object_id = $1)`, item, redHeart).
		Scan(&hearts, &likes); err != nil {
		t.Fatal(err)
	}
	if hearts != 1 || likes != 0 {
		t.Fatalf("after a like: %d red hearts and %d feed_likes rows, want 1 and 0", hearts, likes)
	}
	if got := f.eventsOf(t, events.TypeReactionAdded); got != 1 {
		t.Fatalf("%d reaction.added events after a like, want 1", got)
	}
	if _, err := f.routes.DeleteFeedLike(ctx, api.DeleteFeedLikeRequestObject{Id: item}); err != nil {
		t.Fatal(err)
	}
	if got := f.held(t, item, me); got != 0 {
		t.Fatalf("%d reactions after an unlike, want 0", got)
	}
}

func TestPutFeedReaction_throughTheRoutes(t *testing.T) {
	t.Parallel()
	f := newReactFixture(t)
	item, me := f.item(t, feed.KindTrade), f.cabal.Members[0].ID
	ctx := asUser(t.Context(), me)
	put, err := f.routes.PutFeedReaction(ctx, api.PutFeedReactionRequestObject{Id: item, Emoji: "🎉"})
	if err != nil {
		t.Fatal(err)
	}
	want := api.ReactionSummary{Reactions: []baseapi.Reaction{{Emoji: "🎉", Count: 1, Mine: true}}, Total: 1}
	if got := put.(api.PutFeedReaction200JSONResponse); got.Total != want.Total || len(got.Reactions) != 1 ||
		got.Reactions[0] != want.Reactions[0] {
		t.Fatalf("put = %+v, want %+v", got, want)
	}
	del, err := f.routes.DeleteFeedReaction(ctx, api.DeleteFeedReactionRequestObject{Id: item, Emoji: "🎉"})
	if err != nil {
		t.Fatal(err)
	}
	if got := del.(api.DeleteFeedReaction200JSONResponse); got.Total != 0 || len(got.Reactions) != 0 {
		t.Fatalf("delete = %+v, want no reactions", got)
	}
	bad := api.DeleteFeedReactionRequestObject{Id: item, Emoji: "x"}
	if _, err := f.routes.DeleteFeedReaction(ctx, bad); err == nil {
		t.Fatal("delete of a non-emoji = nil error")
	}
	anon := t.Context()
	putReq := api.PutFeedReactionRequestObject{Id: item, Emoji: "🎉"}
	if _, err := f.routes.PutFeedReaction(anon, putReq); err == nil {
		t.Fatal("put without a caller = nil error")
	}
	delReq := api.DeleteFeedReactionRequestObject{Id: item, Emoji: "🎉"}
	if _, err := f.routes.DeleteFeedReaction(anon, delReq); err == nil {
		t.Fatal("delete without a caller = nil error")
	}
}

type breakOnQuery struct {
	name string
	skip atomic.Int32
}

func (c *breakOnQuery) TraceQueryStart(
	ctx context.Context, conn *pgx.Conn, d pgx.TraceQueryStartData,
) context.Context {
	if strings.Contains(d.SQL, "-- name: "+c.name+" ") && c.skip.Add(-1) < 0 {
		_ = conn.Close(context.WithoutCancel(ctx))
	}
	return ctx
}

func (*breakOnQuery) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestReactFeedItem_failWhenAStoreReadFails(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct {
		query string
		skip  int32
	}{
		"the cap count":                {"CountUserReactionsOnItem", 0},
		"the summary after a reaction": {"ReactionSummaries", 1},
		"the summary after a removal":  {"ReactionSummaries", 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newReactFixture(t)
			item, me := f.item(t, feed.KindTrade), f.cabal.Members[0].ID
			if _, err := f.add(t, item, me, "🎉"); err != nil {
				t.Fatal(err)
			}
			tracer := &breakOnQuery{name: tt.query}
			tracer.skip.Store(tt.skip)
			cfg := f.pool.Config().Copy()
			cfg.ConnConfig.Tracer = tracer
			down, err := pgxpool.NewWithConfig(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(down.Close)
			ctx := asUser(t.Context(), me)
			deps := app.ReactionDeps{
				UoW:     db.New(down, f.gen, f.clock),
				Reads:   down,
				Members: membersOn(down),
				Clock:   f.clock,
				Hints:   f.hints,
			}
			var herr error
			if name == "the summary after a removal" {
				_, herr = app.NewUnreactFeedItemHandler(deps).Handle(
					ctx, app.UnreactFeedItem{FeedObjectID: item, Viewer: me, Emoji: "🎉"})
			} else {
				_, herr = app.NewReactFeedItemHandler(deps).Handle(
					ctx, app.ReactFeedItem{FeedObjectID: item, Viewer: me, Emoji: "👀"})
			}
			if herr == nil {
				t.Errorf("%s: err = nil, want the failed read", name)
			}
		})
	}
}
