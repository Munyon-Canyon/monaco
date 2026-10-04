package social_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestFeedConsumer_isRegistered(t *testing.T) {
	t.Parallel()
	for _, consumer := range social.New(module.Deps{}).Consumers() {
		registered := consumer.Durable == "social_feed" && len(consumer.Handlers) == 3
		if registered && consumer.Handlers[0].Name == "social.feed" {
			return
		}
	}
	t.Fatal("social feed consumer is not registered")
}

func TestFeedConsumer_writesJoinedItemFromTheDeliveryEvent(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal, eventID := f.gen.NewV7(), f.gen.NewV7()
	created := adapters.Feed{Users: f.users, IDs: f.gen}
	joined := adapters.Feed{Bus: testkit.NATS(t).Conn, Users: f.users, IDs: f.gen}
	createdCtx := observability.WithEventID(t.Context(), ids.EventIDFrom(f.gen.NewV7()))
	if err := db.New(f.pool, f.gen, f.clock).Do(createdCtx, func(ctx context.Context, tx db.Tx) error {
		return created.Handle(ctx, tx, events.CabalCreated{
			V: 1, CabalID: cabal, CreatorID: f.alice.UUID(), Name: "Alpha",
		}, f.now)
	}); err != nil {
		t.Fatal(err)
	}
	ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(eventID))
	if err := db.New(f.pool, f.gen, f.clock).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return joined.Joined(ctx, tx, events.CabalMemberJoined{V: 1, CabalID: cabal, UserID: f.bob.UUID()}, f.now)
	}); err != nil {
		t.Fatal(err)
	}
	var ref, title string
	err := f.pool.QueryRow(t.Context(),
		`SELECT ref_id::text, title FROM feed_objects WHERE kind = 'member_joined'`).Scan(&ref, &title)
	if err != nil || ref != eventID.String() || title != "bob joined Alpha" {
		t.Fatalf("join = %q %q, %v", ref, title, err)
	}
}

func TestFeedConsumer_retriesAJoinBeforeItsCabal(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(f.gen.NewV7()))
	err := db.New(f.pool, f.gen, f.clock).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return (adapters.Feed{Users: f.users}).Joined(ctx, tx, events.CabalMemberJoined{
			V: 1, CabalID: f.gen.NewV7(), UserID: f.bob.UUID(), Via: "open",
		}, f.now)
	})
	if got := errs.CodeOf(err); got != errs.CodeFeedItemPending {
		t.Fatalf("join code = %s, want %s", got, errs.CodeFeedItemPending)
	}
}

func TestFeedConsumer_returnsAJoinLookupFailure(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, err := f.pool.Exec(t.Context(), "ALTER TABLE feed_cabals RENAME TO feed_cabals_gone"); err != nil {
		t.Fatal(err)
	}
	ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(f.gen.NewV7()))
	err := db.New(f.pool, f.gen, f.clock).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return (adapters.Feed{Users: f.users}).Joined(ctx, tx, events.CabalMemberJoined{
			V: 1, CabalID: f.gen.NewV7(), UserID: f.bob.UUID(), Via: "open",
		}, f.now)
	})
	if err == nil {
		t.Fatal("join lookup failure = nil")
	}
}

func TestFeedConsumer_returnsJoinedWriteFailures(t *testing.T) {
	t.Parallel()
	for name, prepare := range map[string]func(t *testing.T, f fixture){
		"membership": func(t *testing.T, f fixture) {
			t.Helper()
			if _, err := f.pool.Exec(t.Context(), "ALTER TABLE feed_memberships RENAME TO feed_memberships_gone"); err != nil {
				t.Fatal(err)
			}
		},
		"identity": func(_ *testing.T, f fixture) {
			f.users.Fail("UsersByID", errs.New(errs.CodeDBUnavailable, "test"))
		},
		"cabal": func(t *testing.T, f fixture) {
			t.Helper()
			if _, err := f.pool.Exec(t.Context(), "ALTER TABLE feed_cabals RENAME COLUMN name TO name_gone"); err != nil {
				t.Fatal(err)
			}
		},
		"item": func(t *testing.T, f fixture) {
			t.Helper()
			if _, err := f.pool.Exec(t.Context(), "ALTER TABLE feed_objects RENAME TO feed_objects_gone"); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			cabal := createFeedCabal(t, f)
			prepare(t, f)
			ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(f.gen.NewV7()))
			err := db.New(f.pool, f.gen, f.clock).Do(ctx, func(ctx context.Context, tx db.Tx) error {
				return (adapters.Feed{Users: f.users, IDs: f.gen}).Joined(ctx, tx, events.CabalMemberJoined{
					V: 1, CabalID: cabal, UserID: f.bob.UUID(), Via: "open",
				}, f.now)
			})
			if err == nil {
				t.Fatal("join failure = nil")
			}
		})
	}
}

func TestFeedConsumer_returnsALeftLookupFailure(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, err := f.pool.Exec(t.Context(), "ALTER TABLE feed_cabals RENAME TO feed_cabals_gone"); err != nil {
		t.Fatal(err)
	}
	ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(f.gen.NewV7()))
	err := db.New(f.pool, f.gen, f.clock).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return (adapters.Feed{}).Left(ctx, tx, events.CabalMemberLeft{
			V: 1, CabalID: f.gen.NewV7(), UserID: f.bob.UUID(),
		}, f.now)
	})
	if err == nil {
		t.Fatal("left lookup failure = nil")
	}
}

func TestFeedConsumer_skipsTheCreatorJoinItem(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.gen.NewV7()
	consumer := adapters.Feed{Users: f.users, IDs: f.gen}
	createdCtx := observability.WithEventID(t.Context(), ids.EventIDFrom(f.gen.NewV7()))
	if err := db.New(f.pool, f.gen, f.clock).Do(createdCtx, func(ctx context.Context, tx db.Tx) error {
		return consumer.Handle(ctx, tx, events.CabalCreated{
			V: 1, CabalID: cabal, CreatorID: f.alice.UUID(), Name: "Alpha",
		}, f.now)
	}); err != nil {
		t.Fatal(err)
	}
	joinCtx := observability.WithEventID(t.Context(), ids.EventIDFrom(f.gen.NewV7()))
	if err := db.New(f.pool, f.gen, f.clock).Do(joinCtx, func(ctx context.Context, tx db.Tx) error {
		return consumer.Joined(ctx, tx, events.CabalMemberJoined{
			V: 1, CabalID: cabal, UserID: f.alice.UUID(), Via: "create",
		}, f.now)
	}); err != nil {
		t.Fatal(err)
	}
	var count int
	err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM feed_objects WHERE kind = 'member_joined'`).Scan(&count)
	if err != nil || count != 0 {
		t.Fatalf("creator joins = %d, %v; want 0", count, err)
	}
}

func TestFeedConsumer_keepsJoinHistoryAfterADelayedJoin(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal, joinID, leftID := f.gen.NewV7(), f.gen.NewV7(), f.gen.NewV7()
	consumer := adapters.Feed{Bus: testkit.NATS(t).Conn, Users: f.users, IDs: f.gen}
	createdCtx := observability.WithEventID(t.Context(), ids.EventIDFrom(f.gen.NewV7()))
	if err := db.New(f.pool, f.gen, f.clock).Do(createdCtx, func(ctx context.Context, tx db.Tx) error {
		return consumer.Handle(ctx, tx, events.CabalCreated{
			V: 1, CabalID: cabal, CreatorID: f.alice.UUID(), Name: "Alpha",
		}, f.now)
	}); err != nil {
		t.Fatal(err)
	}
	leftCtx := observability.WithEventID(t.Context(), ids.EventIDFrom(leftID))
	if err := db.New(f.pool, f.gen, f.clock).Do(leftCtx, func(ctx context.Context, tx db.Tx) error {
		return consumer.Left(ctx, tx, events.CabalMemberLeft{V: 1, CabalID: cabal, UserID: f.bob.UUID()}, f.now)
	}); err != nil {
		t.Fatal(err)
	}
	joinCtx := observability.WithEventID(t.Context(), ids.EventIDFrom(joinID))
	if err := db.New(f.pool, f.gen, f.clock).Do(joinCtx, func(ctx context.Context, tx db.Tx) error {
		return consumer.Joined(ctx, tx, events.CabalMemberJoined{
			V: 1, CabalID: cabal, UserID: f.bob.UUID(), Via: "open",
		}, f.now)
	}); err != nil {
		t.Fatal(err)
	}
	var active bool
	var items int
	err := f.pool.QueryRow(t.Context(), `SELECT active, (SELECT count(*) FROM feed_objects WHERE kind = 'member_joined')
		FROM feed_memberships WHERE cabal_id = $1 AND user_id = $2`, cabal, f.bob.UUID()).Scan(&active, &items)
	if err != nil || active || items != 1 {
		t.Fatalf("membership active = %t, items = %d, %v; want false, 1", active, items, err)
	}
}

func TestFeedConsumer_retriesALeaveBeforeItsCabal(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(f.gen.NewV7()))
	err := db.New(f.pool, f.gen, f.clock).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return (adapters.Feed{}).Left(ctx, tx, events.CabalMemberLeft{
			V: 1, CabalID: f.gen.NewV7(), UserID: f.bob.UUID(),
		}, f.now)
	})
	if got := errs.CodeOf(err); got != errs.CodeFeedItemPending {
		t.Fatalf("leave code = %s, want %s", got, errs.CodeFeedItemPending)
	}
}

func createFeedCabal(t *testing.T, f fixture) [16]byte {
	t.Helper()
	cabal := f.gen.NewV7()
	ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(f.gen.NewV7()))
	if err := db.New(f.pool, f.gen, f.clock).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return (adapters.Feed{Users: f.users, IDs: f.gen}).Handle(ctx, tx, events.CabalCreated{
			V: 1, CabalID: cabal, CreatorID: f.alice.UUID(), Name: "Alpha",
		}, f.now)
	}); err != nil {
		t.Fatal(err)
	}
	return cabal
}

func TestFeedConsumer_writesCreatedSnapshot(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	e := adapters.Feed{Users: f.users, IDs: f.gen}
	cabal := f.gen.NewV7()
	event := events.CabalCreated{V: 1, CabalID: cabal, CreatorID: f.alice.UUID(), Name: "Alpha"}
	ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(f.gen.NewV7()))
	err := db.New(f.pool, f.gen, f.clock).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return e.Handle(ctx, tx, event, f.now)
	})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	err = f.pool.QueryRow(t.Context(), `SELECT count(*) FROM feed_objects WHERE ref_id = $1`, cabal).Scan(&n)
	if err != nil || n != 1 {
		t.Fatalf("items = %d, %v; want 1", n, err)
	}
}

func TestFeedConsumer_returnsEachCreatedWriteFailure(t *testing.T) {
	t.Parallel()
	for name, ddl := range map[string]string{
		"cabal":  "ALTER TABLE feed_cabals RENAME TO feed_cabals_gone",
		"member": "ALTER TABLE feed_memberships RENAME TO feed_memberships_gone",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			if _, err := f.pool.Exec(t.Context(), ddl); err != nil {
				t.Fatal(err)
			}
			err := db.New(f.pool, f.gen, f.clock).Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
				return (adapters.Feed{Users: f.users, IDs: f.gen}).Handle(ctx, tx, events.CabalCreated{
					V: 1, CabalID: f.gen.NewV7(), CreatorID: f.alice.UUID(), Name: "Alpha",
				}, f.now)
			})
			if got := errs.CodeOf(err); err == nil || got != errs.CodeInternal {
				t.Fatalf("created feed item = %v (%s)", err, got)
			}
		})
	}
	f := newFixture(t)
	f.users.Fail("UsersByID", errs.New(errs.CodeDBUnavailable, "test"))
	err := db.New(f.pool, f.gen, f.clock).Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return (adapters.Feed{Users: f.users, IDs: f.gen}).Handle(ctx, tx, events.CabalCreated{
			V: 1, CabalID: f.gen.NewV7(), CreatorID: f.alice.UUID(), Name: "Alpha",
		}, f.now)
	})
	if got := errs.CodeOf(err); err == nil || got != errs.CodeDBUnavailable {
		t.Fatalf("identity failure = %v (%s)", err, got)
	}
}
