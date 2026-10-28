package social_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const redHeart = "❤\uFE0F"

func (f likeFixture) react(t *testing.T, item uuid.UUID, user ids.UserID, emoji string, after time.Duration) {
	t.Helper()
	parsed, err := domain.ParseEmoji(emoji)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlc.New(f.pool).AddReaction(t.Context(), sqlc.AddReactionParams{
		FeedObjectID: item, UserID: user.UUID(), Emoji: parsed.String(), CreatedAt: f.clock.Now().Add(after),
	}); err != nil {
		t.Fatal(err)
	}
}

func wantReactions(t *testing.T, label string, got api.FeedItem, want []api.Reaction, total int32) {
	t.Helper()
	if !slices.Equal(got.Reactions, want) || got.ReactionCount != total {
		t.Fatalf("%s: reactions %v, reaction_count %d, want %v and %d",
			label, got.Reactions, got.ReactionCount, want, total)
	}
}

func TestFeedReads_orderReactionsByCountThenFirstReactionAndMarkTheViewers(t *testing.T) {
	t.Parallel()
	f := newLikeFixture(t)
	item := f.item(t, feed.KindProposal)
	a, b, c := f.cabal.Members[0].ID, f.cabal.Members[1].ID, f.outsider
	for i, r := range []struct {
		user  ids.UserID
		emoji string
	}{{a, "🙌"}, {b, "👀"}, {a, "🎉"}, {b, "🎉"}, {c, "🎉"}, {c, "👀"}, {c, "🙌"}, {a, "💪"}} {
		f.react(t, item, r.user, r.emoji, time.Duration(i)*time.Second)
	}
	for _, tt := range []struct {
		viewer ids.UserID
		mine   []bool
	}{{a, []bool{true, true, false, true}}, {b, []bool{true, false, true, false}}} {
		want := []api.Reaction{
			{Emoji: "🎉", Count: 3, Mine: tt.mine[0]},
			{Emoji: "🙌", Count: 2, Mine: tt.mine[1]},
			{Emoji: "👀", Count: 2, Mine: tt.mine[2]},
			{Emoji: "💪", Count: 1, Mine: tt.mine[3]},
		}
		wantReactions(t, "get", f.detail(t, item, tt.viewer), want, 8)
		wantReactions(t, "new", f.listed(t, item, tt.viewer, api.New), want, 8)
		wantReactions(t, "top", f.listed(t, item, tt.viewer, api.Top), want, 8)
	}
	bare := f.item(t, feed.KindTrade)
	wantReactions(t, "no reactions", f.detail(t, bare, a), []api.Reaction{}, 0)
}

func TestFeedReads_returnTheTop20EmojiAndCountEveryReaction(t *testing.T) {
	t.Parallel()
	f := newLikeFixture(t)
	item := f.item(t, feed.KindTrade)
	a, b := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	var want []api.Reaction
	for i := range rune(25) {
		emoji := string(0x1F600 + i)
		f.react(t, item, a, emoji, time.Duration(i)*time.Second)
		if i < 20 {
			want = append(want, api.Reaction{Emoji: emoji, Count: 1, Mine: true})
		}
	}
	last := string(rune(0x1F600 + 24))
	f.react(t, item, b, last, time.Minute)
	want = append([]api.Reaction{{Emoji: last, Count: 2, Mine: true}}, want[:19]...)
	wantReactions(t, "get", f.detail(t, item, a), want, 26)
	wantReactions(t, "new", f.listed(t, item, a, api.New), want, 26)
}

func TestFeedReads_showMigratedLikesAsRedHearts(t *testing.T) {
	t.Parallel()
	f := newLikeFixture(t)
	item := f.item(t, feed.KindTrade)
	a, b := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	for _, user := range []ids.UserID{a, b} {
		if _, err := f.pool.Exec(t.Context(),
			`INSERT INTO feed_likes (feed_object_id, user_id) VALUES ($1, $2)`, item, user.UUID()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(t.Context(), likesCopy(t)); err != nil {
		t.Fatal(err)
	}
	got := f.detail(t, item, a)
	wantReactions(t, "copied", got, []api.Reaction{{Emoji: redHeart, Count: 2, Mine: true}}, 2)
	if got.LikeCount != 2 || !got.LikedByMe {
		t.Fatalf("like_count %d, liked_by_me %v, want 2 and true", got.LikeCount, got.LikedByMe)
	}
	if err := f.remove(t, item, b); err != nil {
		t.Fatal(err)
	}
	if got := f.detail(t, item, b); got.LikeCount != 1 || got.LikedByMe {
		t.Fatalf("after unlike: like_count %d, liked_by_me %v, want 1 and false", got.LikeCount, got.LikedByMe)
	}
	if err := f.put(t, item, f.outsider); err != nil {
		t.Fatal(err)
	}
	wantReactions(t, "liked through the like route", f.detail(t, item, f.outsider),
		[]api.Reaction{{Emoji: redHeart, Count: 2, Mine: true}}, 2)
}

func likesCopy(t *testing.T) string {
	t.Helper()
	paths, err := filepath.Glob("../../../migrations/*_social_feed_reactions.sql")
	if err != nil || len(paths) != 1 {
		t.Fatalf("migration files %v, %v, want one", paths, err)
	}
	raw, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	for stmt := range strings.SplitSeq(string(raw), ";") {
		if stmt = strings.TrimSpace(stmt); strings.HasPrefix(stmt, "INSERT INTO feed_reactions") {
			return stmt
		}
	}
	t.Fatal("the migration has no INSERT INTO feed_reactions")
	return ""
}

type statementCounter struct{ list, summaries atomic.Int32 }

func (c *statementCounter) TraceQueryStart(
	ctx context.Context,
	_ *pgx.Conn,
	d pgx.TraceQueryStartData,
) context.Context {
	switch {
	case strings.Contains(d.SQL, "-- name: ListFeed "):
		c.list.Add(1)
	case strings.Contains(d.SQL, "-- name: ReactionSummaries "):
		c.summaries.Add(1)
	}
	return ctx
}

func (*statementCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestListFeed_readsTheReactionsOfAWholePageInOneQuery(t *testing.T) {
	t.Parallel()
	f := newLikeFixture(t)
	viewer := f.cabal.Members[0].ID
	items, emojis := make([]uuid.UUID, 30), make([]domain.Emoji, 30)
	for i := range rune(30) {
		items[i], emojis[i] = f.item(t, feed.KindTrade), domain.Emoji(string(0x1F600+i))
		f.react(t, items[i], viewer, emojis[i].String(), 0)
	}
	counter := &statementCounter{}
	cfg := f.pool.Config().Copy()
	cfg.ConnConfig.Tracer = counter
	traced, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(traced.Close)
	page, err := app.ListFeed(t.Context(), traced, app.FeedQuery{Viewer: viewer, Limit: 30})
	if err != nil {
		t.Fatal(err)
	}
	if list, summaries := counter.list.Load(), counter.summaries.Load(); list != 1 || summaries != 1 {
		t.Fatalf("statements: %d feed reads and %d summary reads, want 1 and 1", list, summaries)
	}
	if len(page.Items) != 30 {
		t.Fatalf("%d items, want 30", len(page.Items))
	}
	for _, item := range page.Items {
		i := slices.Index(items, item.ID)
		if i < 0 {
			t.Fatalf("item %s was not seeded", item.ID)
		}
		want := []app.Reaction{{Emoji: emojis[i], Count: 1, Mine: true}}
		if !slices.Equal(item.Reactions, want) || item.ReactionCount != 1 {
			t.Fatalf("item %s: reactions %v, count %d, want %v", item.ID, item.Reactions, item.ReactionCount, want)
		}
	}
}

type cancelOnSummaries struct{ cancel context.CancelFunc }

func (c *cancelOnSummaries) TraceQueryStart(
	ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData,
) context.Context {
	if strings.Contains(d.SQL, "-- name: ReactionSummaries ") {
		c.cancel()
	}
	return ctx
}

func (*cancelOnSummaries) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestFeedReads_failWithTheReactionReadsError(t *testing.T) {
	t.Parallel()
	f := newLikeFixture(t)
	item, viewer := f.item(t, feed.KindTrade), f.cabal.Members[0].ID
	tracer := &cancelOnSummaries{}
	cfg := f.pool.Config().Copy()
	cfg.ConnConfig.Tracer = tracer
	down, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(down.Close)
	reads := map[string]struct {
		op   string
		read func(context.Context) error
	}{
		"new": {"social.ListFeed", func(ctx context.Context) error {
			_, err := app.ListFeed(ctx, down, app.FeedQuery{Viewer: viewer, Limit: app.FeedPageDefault})
			return err
		}},
		"top": {"social.ListFeed", func(ctx context.Context) error {
			_, err := app.ListFeed(ctx, down, app.FeedQuery{
				Viewer: viewer, Sort: feed.SortTop, Now: f.clock.Now(), Limit: app.FeedPageDefault,
			})
			return err
		}},
		"get": {"social.GetFeedItem", func(ctx context.Context) error {
			_, err := app.GetFeedItem(ctx, down, viewer, item, app.FeedFilter{})
			return err
		}},
	}
	for name, tt := range reads {
		ctx, cancel := context.WithCancel(t.Context())
		tracer.cancel = cancel
		err := tt.read(ctx)
		cancel()
		var wrapped *errs.Error
		if !errors.As(err, &wrapped) || wrapped.Op != tt.op || !errors.Is(err, context.Canceled) ||
			errs.CodeOf(err) != errs.CodeInternal {
			t.Errorf("%s: err = %v, want %s wrapping the failed reaction read as internal", name, err, tt.op)
		}
	}
}
