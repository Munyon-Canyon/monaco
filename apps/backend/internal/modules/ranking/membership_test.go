package ranking_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type deliverer struct {
	pool  *pgxpool.Pool
	gen   *testkit.IDs
	clock *testkit.Clock
	uow   *db.UnitOfWork
	conn  *bus.Conn
}

func newDeliverer(t *testing.T) deliverer {
	t.Helper()
	g := testkit.NewIDs(618)
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Second))
	pool := testkit.DB(t)
	return deliverer{pool: pool, gen: g, clock: clk, uow: db.New(pool, g, clk)}
}

func (d deliverer) event(t *testing.T, ev events.Event) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	ctx := observability.WithActor(t.Context(), "system:ranking-test")
	err := d.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		if err := tx.Events.Append(ctx, ev); err != nil {
			return err
		}
		return tx.Queries().QueryRow(ctx, `SELECT id FROM events ORDER BY id DESC LIMIT 1`).Scan(&id)
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (d deliverer) deliverAs(
	ctx context.Context, t *testing.T, id uuid.UUID, handler string, ev events.Event,
) (bool, error) {
	t.Helper()
	deps := module.Deps{Pool: d.pool, UoW: d.uow, IDs: d.gen, Clock: d.clock, Bus: d.conn}
	ctx = observability.WithEventID(ctx, ids.EventIDFrom(id))
	for _, c := range ranking.New(deps).Consumers() {
		for _, h := range c.Handlers {
			if h.Name == handler {
				return bus.Deliver(ctx, d.uow, d.clock, h, ids.EventIDFrom(id), ev)
			}
		}
	}
	t.Fatalf("handler %s is not registered", handler)
	return false, nil
}

func (d deliverer) triggers(t *testing.T) []string {
	t.Helper()
	var got []string
	err := d.pool.QueryRow(t.Context(), `SELECT coalesce(array_agg(cabal_id::text || ' ' || reason || ' ' ||
		(created_at = $1)::text ORDER BY id), '{}') FROM ranking_triggers`, d.clock.Now()).Scan(&got)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestMembership_insertsOneTriggerPerEvent(t *testing.T) {
	t.Parallel()
	cabal, user := uuid.NewSHA1(uuid.Nil, []byte("cabal")), uuid.NewSHA1(uuid.Nil, []byte("user"))
	for name, tc := range map[string]struct {
		handler string
		ev      events.Event
		reason  string
	}{
		"join": {
			handler: "ranking.membership", reason: "member_joined",
			ev: events.CabalMemberJoined{V: 1, CabalID: cabal, UserID: user, Role: "member", Via: "open"},
		},
		"leave": {
			handler: "ranking.membership.left", reason: "member_left",
			ev: events.CabalMemberLeft{V: 1, CabalID: cabal, UserID: user},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newDeliverer(t)
			id := d.event(t, tc.ev)
			d.clock.Advance(time.Minute)

			if duplicate, err := d.deliverAs(t.Context(), t, id, tc.handler, tc.ev); err != nil || duplicate {
				t.Fatalf("first delivery = duplicate %t, %v; want it to succeed", duplicate, err)
			}
			if duplicate, err := d.deliverAs(t.Context(), t, id, tc.handler, tc.ev); err != nil || !duplicate {
				t.Fatalf("redelivery = duplicate %t, %v; want a duplicate", duplicate, err)
			}

			want := cabal.String() + " " + tc.reason + " true"
			if got := d.triggers(t); len(got) != 1 || got[0] != want {
				t.Fatalf("triggers = %q, want one %q stamped at the delivery time", got, want)
			}
		})
	}
}
