package bus_test

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type eventLogDB struct {
	pool  *pgxpool.Pool
	clock *testkit.Clock
	ids   *testkit.IDs
	log   bus.EventLog
}

func newEventLogDB(t *testing.T) eventLogDB {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	return eventLogDB{pool: pool, clock: clk, ids: testkit.NewIDs(21), log: bus.NewEventLog(pool, clk)}
}

func (d eventLogDB) insert(
	t *testing.T, aggregateType string, aggregate uuid.UUID, eventType string, age time.Duration, published bool,
) uuid.UUID {
	t.Helper()
	id := d.ids.NewV7()
	createdAt := d.clock.Now().Add(-age)
	var publishedAt *time.Time
	if published {
		publishedAt = &createdAt
	}
	const insert = `INSERT INTO events
  (id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, created_at, published_at)
VALUES ($1, $2, $3, $4, '{"v":1}', 'system', 'test', $5, $6)`
	args := []any{id, aggregateType, aggregate, eventType, createdAt, publishedAt}
	if _, err := d.pool.Exec(t.Context(), insert, args...); err != nil {
		t.Fatal(err)
	}
	return id
}

func eventIDs(rows []bus.EventRow) []uuid.UUID {
	out := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

func TestEventLog_EventsByAggregate_FiltersByTypeInOrder(t *testing.T) {
	t.Parallel()
	d := newEventLogDB(t)
	user, other := d.ids.NewV7(), d.ids.NewV7()
	first := d.insert(t, "user", user, "user.auth_state_changed", 3*time.Minute, true)
	d.insert(t, "user", user, "user.profile_updated", 2*time.Minute, true)
	last := d.insert(t, "user", user, "user.auth_state_changed", time.Minute, false)
	d.insert(t, "user", other, "user.auth_state_changed", time.Minute, true)
	d.insert(t, "cabal", user, "user.auth_state_changed", time.Minute, true)
	got, err := d.log.EventsByAggregate(t.Context(), "user", user, []string{"user.auth_state_changed"}, 10)
	if err != nil || !slices.Equal(eventIDs(got), []uuid.UUID{first, last}) {
		t.Fatalf("EventsByAggregate = %+v, %v", got, err)
	}
	want := bus.EventRow{
		ID: first, AggregateType: "user", AggregateID: user, Type: "user.auth_state_changed",
		Payload: []byte(`{"v": 1}`), ActorType: "system", ActorID: "test",
		CreatedAt: d.clock.Now().Add(-3 * time.Minute),
	}
	head := got[0]
	if head.PublishedAt == nil || got[1].PublishedAt != nil {
		t.Fatalf("published_at = %v, %v", head.PublishedAt, got[1].PublishedAt)
	}
	head.PublishedAt = nil
	if !reflect.DeepEqual(head, want) {
		t.Fatalf("event = %+v, want %+v", head, want)
	}
}

func TestEventLog_EventsByAggregate_WithoutTypesReadsEveryTypeUpToTheLimit(t *testing.T) {
	t.Parallel()
	d := newEventLogDB(t)
	user := d.ids.NewV7()
	first := d.insert(t, "user", user, "user.auth_state_changed", 3*time.Minute, true)
	d.insert(t, "user", user, "user.profile_updated", 2*time.Minute, true)
	d.insert(t, "user", user, "user.auth_state_changed", time.Minute, false)
	all, err := d.log.EventsByAggregate(t.Context(), "user", user, nil, 10)
	if err != nil || len(all) != 3 {
		t.Fatalf("EventsByAggregate without types = %+v, %v", all, err)
	}
	one, err := d.log.EventsByAggregate(t.Context(), "user", user, nil, 1)
	if err != nil || !slices.Equal(eventIDs(one), []uuid.UUID{first}) {
		t.Fatalf("EventsByAggregate limit 1 = %+v, %v", one, err)
	}
}

func TestEventLog_Unpublished_ListsOnlyStaleRows(t *testing.T) {
	t.Parallel()
	d := newEventLogDB(t)
	aggregate := d.ids.NewV7()
	stale := d.insert(t, "swap", aggregate, "trade.submitted", time.Minute, false)
	newer := d.insert(t, "swap", aggregate, "trade.confirmed", 45*time.Second, false)
	d.insert(t, "swap", aggregate, "trade.failed", 5*time.Second, false)
	d.insert(t, "swap", aggregate, "trade.blocked", time.Hour, true)
	got, err := d.log.Unpublished(t.Context(), 30*time.Second, 10)
	if err != nil || len(got) != 2 || got[0].ID != stale || got[1].ID != newer {
		t.Fatalf("Unpublished = %+v, %v", got, err)
	}
	row := got[0]
	if row.AggregateType != "swap" || row.AggregateID != aggregate || row.Type != "trade.submitted" ||
		!row.CreatedAt.Equal(d.clock.Now().Add(-time.Minute)) {
		t.Fatalf("stale event = %+v", row)
	}
	if limited, err := d.log.Unpublished(t.Context(), 30*time.Second, 1); err != nil || len(limited) != 1 {
		t.Fatalf("Unpublished limit 1 = %+v, %v", limited, err)
	}
}

func TestEventLog_CountUnpublished_CountsOnlyStaleRows(t *testing.T) {
	t.Parallel()
	d := newEventLogDB(t)
	aggregate := d.ids.NewV7()
	d.insert(t, "swap", aggregate, "trade.submitted", time.Minute, false)
	d.insert(t, "swap", aggregate, "trade.confirmed", 45*time.Second, false)
	d.insert(t, "swap", aggregate, "trade.failed", 5*time.Second, false)
	d.insert(t, "swap", aggregate, "trade.blocked", time.Hour, true)
	if n, err := d.log.CountUnpublished(t.Context(), 30*time.Second); err != nil || n != 2 {
		t.Fatalf("CountUnpublished = %d, %v", n, err)
	}
	if n, err := d.log.CountUnpublished(t.Context(), 50*time.Second); err != nil || n != 1 {
		t.Fatalf("CountUnpublished over fifty seconds = %d, %v", n, err)
	}
}

func TestEventLog_DatabaseDown_ReportsDBUnavailable(t *testing.T) {
	t.Parallel()
	d := newEventLogDB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.log.EventsByAggregate(ctx, "user", uuid.New(), nil, 1); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Errorf("EventsByAggregate err = %v", err)
	}
	if _, err := d.log.Unpublished(ctx, time.Second, 1); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Errorf("Unpublished err = %v", err)
	}
	if _, err := d.log.CountUnpublished(ctx, time.Second); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Errorf("CountUnpublished err = %v", err)
	}
}

func TestEventLog_QueryCount(t *testing.T) {
	t.Parallel()
	d := newEventLogDB(t)
	aggregate := d.ids.NewV7()
	d.insert(t, "user", aggregate, "user.auth_state_changed", time.Minute, false)
	check := func(name string, call func() error) {
		t.Helper()
		testkit.AssertQueries(t, name, func() {
			if err := call(); err != nil {
				t.Fatal(err)
			}
		})
	}
	check("EventLog EventsByAggregate", func() error {
		_, err := d.log.EventsByAggregate(t.Context(), "user", aggregate, []string{"user.auth_state_changed"}, 100)
		return err
	})
	check(
		"EventLog Unpublished",
		func() error { _, err := d.log.Unpublished(t.Context(), 30*time.Second, 50); return err },
	)
	check(
		"EventLog CountUnpublished",
		func() error { _, err := d.log.CountUnpublished(t.Context(), 30*time.Second); return err },
	)
}
