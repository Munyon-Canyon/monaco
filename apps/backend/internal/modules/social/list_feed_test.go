package social_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (f feedFixture) list(t *testing.T, q app.FeedQuery) app.FeedPage {
	t.Helper()
	if q.Limit == 0 {
		q.Limit = app.FeedPageMax
	}
	page, err := app.ListFeed(t.Context(), f.pool, q)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func idsOf(items []app.FeedItem) []uuid.UUID {
	out := make([]uuid.UUID, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

func sameSet(got, want []uuid.UUID) bool {
	a, b := slices.Clone(got), slices.Clone(want)
	cmp := func(x, y uuid.UUID) int { return strings.Compare(x.String(), y.String()) }
	slices.SortFunc(a, cmp)
	slices.SortFunc(b, cmp)
	return slices.Equal(a, b)
}

func TestFeedQuery_KeysetPagination(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	seeded := make([]uuid.UUID, 0, 75)
	for i := range 75 {
		if i%3 == 0 {
			f.clock.Advance(time.Second)
		}
		seeded = append(seeded, f.item(t))
	}
	var walked []app.FeedItem
	var sizes []int
	q := app.FeedQuery{Limit: 30}
	for {
		page := f.list(t, q)
		walked = append(walked, page.Items...)
		sizes = append(sizes, len(page.Items))
		if page.Next == nil {
			break
		}
		cursor, err := domain.ParseKeyset(page.Next.Encode())
		if err != nil {
			t.Fatal(err)
		}
		q.After = &cursor
	}
	if !slices.Equal(sizes, []int{30, 30, 15}) {
		t.Fatalf("page sizes = %v, want [30 30 15]", sizes)
	}
	if !sameSet(idsOf(walked), seeded) || len(walked) != len(seeded) {
		t.Fatalf("walked %d items, want each of the %d seeded exactly once", len(walked), len(seeded))
	}
	wantNewestFirst(t, walked)
}

func wantNewestFirst(t *testing.T, items []app.FeedItem) {
	t.Helper()
	for i := 1; i < len(items); i++ {
		prev, cur := items[i-1], items[i]
		newer := prev.CreatedAt.After(cur.CreatedAt) ||
			(prev.CreatedAt.Equal(cur.CreatedAt) && prev.ID.String() > cur.ID.String())
		if !newer {
			t.Fatalf("item %d (%s %s) does not sort after item %d (%s %s)",
				i, cur.CreatedAt, cur.ID, i-1, prev.CreatedAt, prev.ID)
		}
	}
}

func TestFeedQuery_Filters(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer, bob, carol := ids.NewUserID(f.gen), ids.NewUserID(f.gen), ids.NewUserID(f.gen)
	alpha, bravo := ids.CabalIDFrom(f.gen.NewV7()), ids.CabalIDFrom(f.gen.NewV7())
	in := func(c ids.CabalID, name string) func(*feed.Item) {
		return func(it *feed.Item) { it.CabalID, it.Payload.CabalName = c, name }
	}
	by := func(u ids.UserID) func(*feed.Item) { return func(it *feed.Item) { it.ActorID = u } }
	trade := f.item(t, in(alpha, "Alpha Cabal"), by(bob))
	tesla := f.item(t, in(bravo, "Bravo Cabal"), by(carol), func(it *feed.Item) {
		it.Payload.Symbol, it.Payload.AssetName = "TSLAx", "Tesla"
	})
	proposal := f.item(t, in(alpha, "Alpha Cabal"), by(carol), func(it *feed.Item) {
		it.Kind, it.Status, it.Body = feed.KindProposal, "open", "Earnings beat expectations again"
	})
	move := f.item(t, func(it *feed.Item) {
		it.Kind, it.Payload = feed.KindPriceMove, feed.Payload{Symbol: "AAPLx", ChangeBps: 1043}
	})
	joined := f.item(t, in(bravo, "Bravo Cabal"), by(bob), func(it *feed.Item) {
		it.Kind, it.Payload.Symbol = feed.KindMemberJoined, ""
	})
	follow := `INSERT INTO follows (id, follower_id, followee_id, created_at, deleted_at) VALUES ($1, $2, $3, now(), $4)`
	if _, err := f.pool.Exec(t.Context(), follow, f.gen.NewV7(), viewer.UUID(), bob.UUID(), nil); err != nil {
		t.Fatal(err)
	}
	unfollowed := f.clock.Now()
	if _, err := f.pool.Exec(t.Context(), follow, f.gen.NewV7(), viewer.UUID(), carol.UUID(), unfollowed); err != nil {
		t.Fatal(err)
	}
	all := []uuid.UUID{trade, tesla, proposal, move, joined}
	for name, c := range map[string]struct {
		filter app.FeedFilter
		want   []uuid.UUID
	}{
		"none": {app.FeedFilter{}, []uuid.UUID{trade, tesla, proposal, move, joined}},
		"kind": {app.FeedFilter{Kinds: []feed.Kind{feed.KindTrade}}, []uuid.UUID{trade, tesla}},
		"kinds": {
			app.FeedFilter{Kinds: []feed.Kind{feed.KindProposal, feed.KindPriceMove}}, []uuid.UUID{proposal, move},
		},
		"cabal_id":        {app.FeedFilter{CabalID: bravo}, []uuid.UUID{tesla, joined}},
		"symbol":          {app.FeedFilter{Symbol: "tslax"}, []uuid.UUID{tesla}},
		"q symbol":        {app.FeedFilter{Q: "AAPLx"}, []uuid.UUID{trade, proposal, move}},
		"q cabal name":    {app.FeedFilter{Q: "bravo"}, []uuid.UUID{tesla, joined}},
		"q stemmed body":  {app.FeedFilter{Q: "earning"}, []uuid.UUID{proposal}},
		"q no match":      {app.FeedFilter{Q: "dogecoin"}, nil},
		"scope following": {app.FeedFilter{Scope: feed.ScopeFollowing}, []uuid.UUID{trade, joined}},
		"following and kind": {
			app.FeedFilter{Scope: feed.ScopeFollowing, Kinds: []feed.Kind{feed.KindTrade}}, []uuid.UUID{trade},
		},
	} {
		got := f.list(t, app.FeedQuery{Viewer: viewer, Filter: c.filter}).Items
		if !sameSet(idsOf(got), c.want) {
			t.Errorf("%s: got %v, want %v", name, idsOf(got), c.want)
		}
		f.wantVisibleExactly(t, viewer, c.filter, all, c.want)
	}
}

func (f feedFixture) wantVisibleExactly(t *testing.T, viewer ids.UserID, filter app.FeedFilter, all, want []uuid.UUID) {
	t.Helper()
	for _, id := range all {
		view, err := app.GetFeedItem(t.Context(), f.pool, viewer, id, filter)
		if err != nil {
			t.Fatal(err)
		}
		if view.Item.ID != id || view.Visible != slices.Contains(want, id) {
			t.Errorf("%+v: item %s visible = %v, want %v", filter, id, view.Visible, slices.Contains(want, id))
		}
	}
}

func TestFeedQuery_rendersEachItemFromItsSnapshot(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	bob, cabal := ids.NewUserID(f.gen), ids.CabalIDFrom(f.gen.NewV7())
	ref := f.gen.NewV7()
	id := f.item(t, func(it *feed.Item) {
		it.Kind, it.RefID, it.CabalID, it.ActorID, it.Body = feed.KindPriceMove, ref, cabal, bob, "why"
		it.Payload = feed.Payload{
			Symbol: "TSLAx", AssetName: "Tesla", ChangeBps: -507, MarkMicros: money.MicrosFromUint64(250_000_000),
			PrevClose: money.MicrosFromUint64(263_400_000),
		}
	})
	got := f.list(t, app.FeedQuery{}).Items
	want := app.FeedItem{
		ID: id, Kind: feed.KindPriceMove, RefType: feed.RefAssetPriceMoves, RefID: ref, CabalID: cabal, ActorID: bob,
		Symbol: "TSLAx", Title: "TSLAx is down 5.07% today", Detail: "$250.00, previous close $263.40", Body: "why",
		Tone: feed.ToneNegative, CreatedAt: f.clock.Now(), UpdatedAt: f.clock.Now(),
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("items = %+v, want [%+v]", got, want)
	}
}

func TestFeedQuery_refusesALimitOutsideOneToFifty(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	for _, limit := range []int{-1, 0, app.FeedPageMax + 1} {
		_, err := app.ListFeed(t.Context(), f.pool, app.FeedQuery{Limit: limit})
		wantCode(t, err, errs.CodeInvalidInput)
	}
}

func TestFeedQuery_failsOnAPayloadItCannotRead(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO feed_objects (id, kind, ref_type, ref_id, title, payload,
		created_at, updated_at) VALUES ($1, 'trade', 'swaps', $2, 't', '{"usdc_micros": 1.5}', now(), now())`,
		f.gen.NewV7(), f.gen.NewV7())
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.ListFeed(t.Context(), f.pool, app.FeedQuery{Limit: 1})
	wantCode(t, err, errs.CodeInternal)
}

func TestFeedQuery_reportsADatabaseFailure(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := app.ListFeed(ctx, f.pool, app.FeedQuery{Limit: 1}); err == nil {
		t.Fatal("ListFeed on a cancelled context succeeded")
	}
}

type lastQuery struct {
	sql  string
	args []any
}

func (q *lastQuery) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	q.sql, q.args = data.SQL, data.Args
	return ctx
}

func (*lastQuery) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (f feedFixture) traced(t *testing.T) (*pgxpool.Pool, *lastQuery) {
	t.Helper()
	last := &lastQuery{}
	cfg := f.pool.Config()
	cfg.ConnConfig.Tracer = last
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, last
}

func TestFeedQuery_defaultPageReadsTheNewestIndexWithoutASeqScan(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO feed_objects (id, kind, ref_type, ref_id, cabal_name, symbol,
		title, payload, created_at, updated_at)
		SELECT gen_random_uuid(), 'trade', 'swaps', gen_random_uuid(), 'Cabal ' || (n % 50), 'SYM' || (n % 40),
			'Cabal bought $' || n || ' of SYM', '{}', $1::timestamptz - n * interval '1 second', $1
		FROM generate_series(1, 10000) AS n`, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `ANALYZE feed_objects`); err != nil {
		t.Fatal(err)
	}
	traced, last := f.traced(t)
	page, err := app.ListFeed(
		t.Context(),
		traced,
		app.FeedQuery{Viewer: ids.NewUserID(f.gen), Limit: app.FeedPageDefault},
	)
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
	if strings.Contains(text, "Seq Scan") || !strings.Contains(text, "feed_objects_newest_idx") {
		t.Fatalf("default feed page plan:\n%s\nwant an index scan on feed_objects_newest_idx and no seq scan", text)
	}
}

func TestGetFeedItem_namesAnUnknownItem(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	_, err := app.GetFeedItem(t.Context(), f.pool, ids.NewUserID(f.gen), f.gen.NewV7(), app.FeedFilter{})
	wantCode(t, err, errs.CodeFeedItemNotFound)
}

func TestGetFeedItem_failsOnAPayloadItCannotReadOrADatabaseFailure(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	id := f.gen.NewV7()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO feed_objects (id, kind, ref_type, ref_id, title, payload,
		created_at, updated_at) VALUES ($1, 'trade', 'swaps', $2, 't', '{"usdc_micros": 1.5}', now(), now())`,
		id, f.gen.NewV7())
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.GetFeedItem(t.Context(), f.pool, ids.NewUserID(f.gen), id, app.FeedFilter{})
	wantCode(t, err, errs.CodeInternal)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := app.GetFeedItem(ctx, f.pool, ids.NewUserID(f.gen), id, app.FeedFilter{}); err == nil {
		t.Fatal("GetFeedItem on a cancelled context succeeded")
	}
}

func (f feedFixture) commented(t *testing.T, count int32, edits ...func(*feed.Item)) uuid.UUID {
	t.Helper()
	id := f.item(t, edits...)
	if _, err := f.pool.Exec(
		t.Context(),
		`UPDATE feed_objects SET comment_count = $1 WHERE id = $2`,
		count,
		id,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f feedFixture) walkTop(t *testing.T, now time.Time, limit int) []app.FeedItem {
	t.Helper()
	q := app.FeedQuery{Sort: feed.SortTop, Now: now, Limit: limit}
	var walked []app.FeedItem
	for {
		page := f.list(t, q)
		walked = append(walked, page.Items...)
		if page.Next == nil {
			return walked
		}
		cursor, err := domain.ParseRankedKeyset(page.Next.Encode())
		if err != nil {
			t.Fatal(err)
		}
		q.After = &cursor
	}
}

func wantRankedByCommentsThenNewest(t *testing.T, items []app.FeedItem) {
	t.Helper()
	for i := 1; i < len(items); i++ {
		prev, cur := items[i-1], items[i]
		if prev.CommentCount != cur.CommentCount {
			if prev.CommentCount < cur.CommentCount {
				t.Fatalf("item %d has %d comments after item %d with %d", i, cur.CommentCount, i-1, prev.CommentCount)
			}
			continue
		}
		wantNewestFirst(t, items[i-1:i+1])
	}
}

func TestFeedQuery_TopPaginatesTiesWithNoGapOrDuplicate(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	const window = 24 * time.Hour
	stale := f.commented(t, 99)
	f.clock.Advance(time.Hour)
	seeded := make([]uuid.UUID, 0, 60)
	for i := range 60 {
		if i%4 == 0 {
			f.clock.Advance(time.Second)
		}
		seeded = append(seeded, f.commented(t, int32(i%3)))
	}
	f.clock.Advance(window - time.Hour - 14*time.Second)
	walked := f.walkTop(t, f.clock.Now(), 7)
	if len(walked) != 60 || !sameSet(idsOf(walked), seeded) {
		t.Fatalf("walked %d items, want each of the 60 in-window items exactly once and not %s", len(walked), stale)
	}
	wantRankedByCommentsThenNewest(t, walked)
	if walked[0].CommentCount != 2 || walked[len(walked)-1].CommentCount != 0 {
		t.Fatalf("counts run %d to %d, want 2 down to 0", walked[0].CommentCount, walked[len(walked)-1].CommentCount)
	}
}

func TestFeedQuery_TopKeepsAnItemUntilTheWindowPassesIt(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	edge := f.item(t)
	created := f.clock.Now()
	for name, c := range map[string]struct {
		now  time.Time
		want int
	}{
		"inside":      {created.Add(feed.TopWindow - time.Microsecond), 1},
		"on the edge": {created.Add(feed.TopWindow), 1},
		"past":        {created.Add(feed.TopWindow + time.Microsecond), 0},
	} {
		page := f.list(t, app.FeedQuery{Sort: feed.SortTop, Now: c.now})
		if len(page.Items) != c.want || (c.want == 1 && page.Items[0].ID != edge) {
			t.Errorf("%s: read %d items, want %d", name, len(page.Items), c.want)
		}
	}
}

func TestFeedQuery_TopHonoursTheFilters(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	trade := f.commented(t, 5)
	proposal := f.commented(t, 1, func(it *feed.Item) { it.Kind, it.Status = feed.KindProposal, "open" })
	page := f.list(t, app.FeedQuery{
		Sort: feed.SortTop, Now: f.clock.Now(), Filter: app.FeedFilter{Kinds: []feed.Kind{feed.KindProposal}},
	})
	if got := idsOf(page.Items); !slices.Equal(got, []uuid.UUID{proposal}) {
		t.Fatalf("top proposals = %v, want only %s and not the busier trade %s", got, proposal, trade)
	}
}

func TestFeedQuery_TopReportsADatabaseFailure(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := app.ListFeed(
		ctx,
		f.pool,
		app.FeedQuery{Sort: feed.SortTop, Now: f.clock.Now(), Limit: 5},
	); err == nil {
		t.Fatal("ListFeed on a cancelled context succeeded")
	}
}
