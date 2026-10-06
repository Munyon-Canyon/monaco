package ranking_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const hintsHandler = "ranking.hints"

func TestHints_publishesOneHintPerRunWithOnlyTheRunAndItsTime(t *testing.T) {
	t.Parallel()
	nats := testkit.NATS(t)
	d := newDeliverer(t)
	d.conn = nats.Conn
	b := seedBoards(t, d, true)
	sub := testkit.SubscribeCore(t, nats, "hint."+boardsHint)
	ev := events.RankingSnapshotWritten{
		V: 1, RunID: b.run, AsOf: b.done, PricesAsOf: b.done, ComputedAt: b.done, RowsWritten: 15, CabalsExcluded: 2,
	}
	id := d.event(t, ev)

	if duplicate, err := d.deliverAs(t.Context(), t, id, hintsHandler, ev); err != nil || duplicate {
		t.Fatalf("first delivery = duplicate %t, %v; want it to succeed", duplicate, err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(sub.Next(t), &got); err != nil {
		t.Fatalf("the hint is not an object: %v", err)
	}
	keys := make([]string, 0, len(got))
	for key := range got {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	var runID string
	var computedAt time.Time
	if err := json.Unmarshal(got["run_id"], &runID); err != nil || runID != b.run.String() ||
		json.Unmarshal(got["computed_at"], &computedAt) != nil || !computedAt.Equal(b.done) ||
		!slices.Equal(keys, []string{"computed_at", "run_id"}) {
		t.Fatalf("hint = %v, want only run %s computed at %s", got, b.run, b.done)
	}
	wantNoHint(t, nats.Conn, sub)

	if duplicate, err := d.deliverAs(t.Context(), t, id, hintsHandler, ev); err != nil || !duplicate {
		t.Fatalf("redelivery = duplicate %t, %v; want a duplicate", duplicate, err)
	}
	wantNoHint(t, nats.Conn, sub)
}

func TestHints_publishesNothingWhenTheDeliveryRollsBack(t *testing.T) {
	t.Parallel()
	nats := testkit.NATS(t)
	d := newDeliverer(t)
	d.conn = nats.Conn
	b := seedBoards(t, d, true)
	sub := testkit.SubscribeCore(t, nats, "hint."+boardsHint)
	ev := events.RankingSnapshotWritten{V: 1, RunID: b.run, ComputedAt: b.done}
	id := d.event(t, ev)
	if _, err := d.pool.Exec(t.Context(), `ALTER TABLE event_deliveries RENAME TO gone`); err != nil {
		t.Fatal(err)
	}

	if _, err := d.deliverAs(t.Context(), t, id, hintsHandler, ev); err == nil {
		t.Fatal("delivery with event_deliveries missing succeeded; want an error")
	}
	wantNoHint(t, nats.Conn, sub)
}
