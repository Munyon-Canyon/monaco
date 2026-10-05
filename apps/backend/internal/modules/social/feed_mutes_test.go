package social_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (f feedFixture) setMute(t *testing.T, viewer ids.UserID, typ, id string, muted bool) {
	t.Helper()
	query := `INSERT INTO feed_mutes (user_id, target_type, target_id, created_at) VALUES ($1, $2, $3, now())`
	if !muted {
		query = `DELETE FROM feed_mutes WHERE user_id = $1 AND target_type = $2 AND target_id = $3`
	}
	if _, err := f.pool.Exec(t.Context(), query, viewer.UUID(), typ, id); err != nil {
		t.Fatal(err)
	}
}

func TestFeedMutes_EachTargetType(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer, other := ids.NewUserID(f.gen), ids.NewUserID(f.gen)
	alpha, asset, actor := ids.CabalIDFrom(f.gen.NewV7()), f.gen.NewV7(), ids.NewUserID(f.gen)
	target := f.item(t, func(it *feed.Item) { it.CabalID, it.AssetID, it.ActorID = alpha, asset, actor })
	bystander := f.item(t, func(it *feed.Item) { it.Kind = feed.KindPriceMove })
	all := []uuid.UUID{target, bystander}
	for _, tt := range []struct {
		typ, id string
		want    []uuid.UUID
	}{
		{"kind", "trade", []uuid.UUID{bystander}},
		{"cabal", alpha.String(), []uuid.UUID{bystander}},
		{"asset", asset.String(), []uuid.UUID{bystander}},
		{"user", actor.String(), []uuid.UUID{bystander}},
		{"item", target.String(), []uuid.UUID{bystander}},
		{"kind", "price_move", []uuid.UUID{target}},
	} {
		f.setMute(t, viewer, tt.typ, tt.id, true)
		got := f.list(t, app.FeedQuery{Viewer: viewer}).Items
		if !sameSet(idsOf(got), tt.want) {
			t.Errorf("mute %s %s: got %v, want %v", tt.typ, tt.id, idsOf(got), tt.want)
		}
		f.wantVisibleExactly(t, viewer, app.FeedFilter{}, all, tt.want)
		if got := f.list(t, app.FeedQuery{Viewer: other}).Items; !sameSet(idsOf(got), all) {
			t.Errorf("mute %s %s hid items from another viewer: %v", tt.typ, tt.id, idsOf(got))
		}
		f.setMute(t, viewer, tt.typ, tt.id, false)
		if got := f.list(t, app.FeedQuery{Viewer: viewer}).Items; !sameSet(idsOf(got), all) {
			t.Errorf("unmute %s %s: got %v, want %v", tt.typ, tt.id, idsOf(got), all)
		}
		f.wantVisibleExactly(t, viewer, app.FeedFilter{}, all, all)
	}
}

func (f feedFixture) membership(t *testing.T, cabal ids.CabalID, user ids.UserID, joined bool) {
	t.Helper()
	q := sqlc.New(f.pool)
	if err := q.UpsertFeedCabal(t.Context(), sqlc.UpsertFeedCabalParams{
		CabalID: cabal.UUID(), Name: "Alpha", At: f.clock.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	var err error
	if joined {
		_, err = q.UpsertFeedMembership(t.Context(), sqlc.UpsertFeedMembershipParams{
			CabalID: cabal.UUID(), UserID: user.UUID(), JoinedAt: f.clock.Now(), EventID: f.gen.NewV7(),
		})
	} else {
		_, err = q.DeleteFeedMembership(t.Context(), sqlc.DeleteFeedMembershipParams{
			CabalID: cabal.UUID(), UserID: user.UUID(), EventID: f.gen.NewV7(), At: f.clock.Now(),
		})
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestFeedQuery_ScopeMine(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer := ids.NewUserID(f.gen)
	alpha, bravo := ids.CabalIDFrom(f.gen.NewV7()), ids.CabalIDFrom(f.gen.NewV7())
	in := func(c ids.CabalID) func(*feed.Item) { return func(it *feed.Item) { it.CabalID = c } }
	mine := f.item(t, in(alpha))
	elsewhere := f.item(t, in(bravo))
	f.item(t, in(alpha), func(it *feed.Item) { it.Kind = feed.KindPriceMove })
	noCabal := f.item(t)
	all := []uuid.UUID{mine, elsewhere, noCabal}
	filter := app.FeedFilter{Scope: feed.ScopeMine}
	check := func(step string, want []uuid.UUID) {
		t.Helper()
		if got := f.list(t, app.FeedQuery{Viewer: viewer, Filter: filter}).Items; !sameSet(idsOf(got), want) {
			t.Errorf("%s: got %v, want %v", step, idsOf(got), want)
		}
		f.wantVisibleExactly(t, viewer, filter, all, want)
	}
	check("before joining", nil)
	f.membership(t, alpha, viewer, true)
	check("after a join", []uuid.UUID{mine})
	f.membership(t, alpha, viewer, false)
	check("after a leave", nil)
}

func TestFeedQuery_defaultPageWithFiftyMutesKeepsTheNewestIndex(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer := ids.NewUserID(f.gen)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO feed_objects (id, kind, ref_type, ref_id, cabal_name, symbol,
		title, payload, created_at, updated_at)
		SELECT gen_random_uuid(), 'trade', 'swaps', gen_random_uuid(), 'Cabal ' || (n % 50), 'SYM' || (n % 40),
			'Cabal bought $' || n || ' of SYM', '{}', $1::timestamptz - n * interval '1 second', $1
		FROM generate_series(1, 10000) AS n`, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.pool.Exec(t.Context(), `INSERT INTO feed_mutes (user_id, target_type, target_id, created_at)
		SELECT $1, 'asset', gen_random_uuid()::text, now() FROM generate_series(1, 50)`, viewer.UUID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `ANALYZE feed_objects, feed_mutes`); err != nil {
		t.Fatal(err)
	}
	traced, last := f.traced(t)
	page, err := app.ListFeed(t.Context(), traced, app.FeedQuery{Viewer: viewer, Limit: app.FeedPageDefault})
	if err != nil || len(page.Items) != app.FeedPageDefault {
		t.Fatalf("default page = %d items, %v", len(page.Items), err)
	}
	rows, err := f.pool.Query(t.Context(), "EXPLAIN "+last.sql, last.args...)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(plan, "\n")
	if strings.Contains(text, "Seq Scan on feed_objects") || !strings.Contains(text, "feed_objects_newest_idx") {
		t.Fatalf("default feed page plan with 50 mutes:\n%s\nwant an index scan on feed_objects_newest_idx", text)
	}
}
