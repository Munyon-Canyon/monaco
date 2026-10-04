package social_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestFeedConsumer_isRegistered(t *testing.T) {
	t.Parallel()
	for _, consumer := range social.New(module.Deps{}).Consumers() {
		registered := consumer.Durable == "social_feed" && len(consumer.Handlers) == 1
		if registered && consumer.Handlers[0].Name == "social.feed" {
			return
		}
	}
	t.Fatal("social feed consumer is not registered")
}

func TestFeedConsumer_writesCreatedSnapshot(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	e := adapters.Feed{Users: f.users, IDs: f.gen}
	cabal := f.gen.NewV7()
	event := events.CabalCreated{V: 1, CabalID: cabal, CreatorID: f.alice.UUID(), Name: "Alpha"}
	err := db.New(f.pool, f.gen, f.clock).Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
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
