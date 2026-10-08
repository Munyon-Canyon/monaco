package bus_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/bucket"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (d eventLogDB) countable(t *testing.T, eventType string, at time.Time, payload map[string]any) {
	t.Helper()
	payload["v"] = 1
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.pool.Exec(t.Context(), `INSERT INTO events
		(id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, created_at)
		VALUES ($1, 'test', $2, $3, $4, 'system', 'test', $5)`,
		d.ids.NewV7(), d.ids.NewV7(), eventType, raw, at); err != nil {
		t.Fatal(err)
	}
}

func sept(day, hour int) time.Time { return time.Date(2026, 9, day, hour, 0, 0, 0, time.UTC) }

func seedCountable(t *testing.T) eventLogDB {
	t.Helper()
	d := newEventLogDB(t)
	d.countable(t, "follow.created", sept(1, 9), map[string]any{"source": "contacts"})
	d.countable(t, "follow.created", sept(1, 10), map[string]any{"source": "contacts"})
	d.countable(t, "follow.created", sept(1, 11), map[string]any{"source": "search"})
	d.countable(t, "follow.created", sept(8, 11), map[string]any{"source": "search"})
	d.countable(t, "follow.created", sept(30, 11), map[string]any{"source": "search"})
	d.countable(t, "follow.removed", sept(1, 12), map[string]any{})
	d.countable(t, "comment.created", sept(2, 1), map[string]any{"parent_comment_id": nil})
	d.countable(t, "comment.created", sept(2, 2), map[string]any{"parent_comment_id": "c1"})
	d.countable(t, "comment.created", sept(2, 3), map[string]any{"parent_comment_id": "c2"})
	return d
}

func TestEventCounts_GroupsByPayloadKeyPerBucket(t *testing.T) {
	t.Parallel()
	d := seedCountable(t)
	counts := bus.NewEventCounts(d.pool)
	tests := map[string]struct {
		query bus.CountQuery
		want  []bus.EventCount
	}{
		"by source per day": {
			bus.CountQuery{
				Types: []string{
					"follow.created",
				},
				From:    sept(1, 0),
				To:      sept(15, 0),
				Size:    bucket.Day,
				GroupBy: "source",
			},
			[]bus.EventCount{
				{Start: sept(1, 0), Type: "follow.created", Group: "contacts", Count: 2},
				{Start: sept(1, 0), Type: "follow.created", Group: "search", Count: 1},
				{Start: sept(8, 0), Type: "follow.created", Group: "search", Count: 1},
			},
		},
		"by source per week": {
			bus.CountQuery{
				Types: []string{
					"follow.created",
				},
				From:    sept(1, 0),
				To:      sept(15, 0),
				Size:    bucket.Week,
				GroupBy: "source",
			},
			[]bus.EventCount{
				{
					Start: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
					Type:  "follow.created",
					Group: "contacts",
					Count: 2,
				},
				{
					Start: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
					Type:  "follow.created",
					Group: "search",
					Count: 1,
				},
				{Start: sept(7, 0), Type: "follow.created", Group: "search", Count: 1},
			},
		},
		"several types without a group": {
			bus.CountQuery{
				Types: []string{"follow.created", "follow.removed"}, From: sept(1, 0), To: sept(2, 0), Size: bucket.Day,
			},
			[]bus.EventCount{
				{Start: sept(1, 0), Type: "follow.created", Count: 3},
				{Start: sept(1, 0), Type: "follow.removed", Count: 1},
			},
		},
		"only replies": {
			bus.CountQuery{
				Types: []string{"comment.created"}, From: sept(1, 0), To: sept(15, 0), Size: bucket.Day,
				Present: "parent_comment_id",
			},
			[]bus.EventCount{{Start: sept(2, 0), Type: "comment.created", Count: 2}},
		},
		"no types": {
			bus.CountQuery{From: sept(1, 0), To: sept(15, 0), Size: bucket.Day},
			[]bus.EventCount{},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := counts.CountEvents(t.Context(), tt.query)
			if err != nil || !slices.Equal(got, tt.want) {
				t.Fatalf("CountEvents() = %+v, %v, want %+v", got, err, tt.want)
			}
		})
	}
}

func TestEventCounts_FailOnACancelledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := bus.NewEventCounts(testkit.DB(t)).CountEvents(ctx, bus.CountQuery{}); err == nil {
		t.Fatal("CountEvents() error = nil on a cancelled context")
	}
}
