package social_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f feedFixture) ban(t *testing.T, cabal ids.CabalID) error {
	t.Helper()
	feedConsumer := adapters.Feed{Bus: testkit.NATS(t).Conn}
	ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(f.gen.NewV7()))
	return db.New(f.pool, f.gen, f.clock).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return feedConsumer.CabalBanned(ctx, tx, events.CabalBanned{V: 1, CabalID: cabal.UUID()}, f.clock.Now())
	})
}

func TestCabalBanned_HidesTheCabalsFeedItemsFromEveryRead(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer := ids.NewUserID(f.gen)
	banned, kept := ids.CabalIDFrom(f.gen.NewV7()), ids.CabalIDFrom(f.gen.NewV7())
	in := func(c ids.CabalID) func(*feed.Item) { return func(it *feed.Item) { it.CabalID = c } }
	gone, stays := f.item(t, in(banned)), f.item(t, in(kept))
	for range 2 {
		if err := f.ban(t, banned); err != nil {
			t.Fatal(err)
		}
	}
	for _, sort := range []feed.Sort{feed.SortNew, feed.SortTop} {
		q := app.FeedQuery{Viewer: viewer, Sort: sort, Now: f.clock.Now()}
		if got := idsOf(f.list(t, q).Items); !sameSet(got, []uuid.UUID{stays}) {
			t.Errorf("sort %v lists %v, want only %s", sort, got, stays)
		}
	}
	f.wantVisibleExactly(t, viewer, app.FeedFilter{}, []uuid.UUID{gone, stays}, []uuid.UUID{stays})
}

func TestCabalBanned_ReportsAFailedWrite(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE feed_cabals RENAME TO feed_cabals_gone`); err != nil {
		t.Fatal(err)
	}
	if err := f.ban(t, ids.CabalIDFrom(f.gen.NewV7())); err == nil {
		t.Fatal("ban with the feed table gone = nil error")
	}
}

func TestCabalBanned_HidesAnItemThatArrivesAfterTheBan(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	ban := func(ctx context.Context, tx db.Tx) error {
		return r.feed.CabalBanned(ctx, tx, events.CabalBanned{V: 1, CabalID: r.cabal}, r.clock.Now())
	}
	if err := r.deliver(t, ban); err != nil {
		t.Fatal(err)
	}
	if err := r.deliverTrade(t, r.confirmed("buy", "proposal")); err != nil {
		t.Fatal(err)
	}
	if got := len(r.tradeRows(t)); got != 1 {
		t.Fatalf("trade items = %d, want the late one stored", got)
	}
	f := feedFixture{pool: r.pool, gen: r.gen, clock: r.clock}
	viewer := ids.NewUserID(r.gen)
	for _, sort := range []feed.Sort{feed.SortNew, feed.SortTop} {
		if got := f.list(t, app.FeedQuery{Viewer: viewer, Sort: sort, Now: r.clock.Now()}).Items; len(got) != 0 {
			t.Errorf("sort %v lists %v, want an empty feed", sort, idsOf(got))
		}
	}
	var item uuid.UUID
	if err := r.pool.QueryRow(t.Context(), `SELECT id FROM feed_objects WHERE kind = 'trade'`).Scan(&item); err != nil {
		t.Fatal(err)
	}
	f.wantVisibleExactly(t, viewer, app.FeedFilter{}, []uuid.UUID{item}, nil)
}
