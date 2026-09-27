package db_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

func (h *harness) appendEvents(t *testing.T, n int) []uuid.UUID {
	t.Helper()
	ids := make([]uuid.UUID, 0, n)
	for range n {
		err := h.uow.Do(h.ctx(t, "user:u1"), func(ctx context.Context, tx db.Tx) error {
			return tx.Events.Append(ctx, pinged(h.ids))
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := h.pool.Query(t.Context(), `SELECT id FROM events ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids[len(ids)-n:]
}

func (h *harness) unpublished(t *testing.T) []uuid.UUID {
	t.Helper()
	rows, err := h.pool.Query(t.Context(), `SELECT id FROM events WHERE published_at IS NULL ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func (h *harness) publishedAt(t *testing.T, id uuid.UUID) time.Time {
	t.Helper()
	var at time.Time
	if err := h.pool.QueryRow(t.Context(), `SELECT published_at FROM events WHERE id = $1`, id).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

func recording(seen *[]uuid.UUID) func(context.Context, db.OutboxRow) error {
	return func(_ context.Context, row db.OutboxRow) error {
		*seen = append(*seen, row.ID)
		return nil
	}
}

func TestDrain_publishesInIDOrderThenMarksAndCommitsTheBatch(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	want := h.appendEvents(t, 3)
	outbox := db.NewOutbox(h.pool, h.clock)
	var seen []uuid.UUID

	b, err := outbox.Drain(t.Context(), 3, recording(&seen))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(seen, want) || !slices.Equal(b.Published, want) || b.Failed != nil || !b.Full {
		t.Fatalf("drain saw %v published %v failed %v full %v, want %v in order, nil, full",
			seen, b.Published, b.Failed, b.Full, want)
	}
	if left := h.unpublished(t); len(left) != 0 {
		t.Fatalf("%d rows still unpublished after a committed batch: %v", len(left), left)
	}
	if at := h.publishedAt(t, want[0]); !at.Equal(h.clock.Now()) {
		t.Fatalf("published_at = %v, want the injected clock %v", at, h.clock.Now())
	}
	b, err = outbox.Drain(t.Context(), 3, recording(&seen))
	if err != nil || len(b.Published) != 0 || b.Full {
		t.Fatalf("second drain = %+v, %v; want an empty, not full batch", b, err)
	}
}

func TestDrain_reportsAPartialBatchAsNotFull(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.appendEvents(t, 2)
	var seen []uuid.UUID
	b, err := db.NewOutbox(h.pool, h.clock).Drain(t.Context(), 100, recording(&seen))
	if err != nil || len(b.Published) != 2 || b.Full {
		t.Fatalf("drain of 2 rows with limit 100 = %+v, %v; want 2 published and not full", b, err)
	}
}

func TestDrain_aPublishFailureCommitsTheAckedPrefixAndKeepsTheRest(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ids := h.appendEvents(t, 3)
	full := errs.New(errs.CodeUpstreamUnavailable, "bus.Publish")
	b, err := db.NewOutbox(h.pool, h.clock).Drain(t.Context(), 100, func(_ context.Context, row db.OutboxRow) error {
		if row.ID == ids[1] {
			return full
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(b.Published, ids[:1]) || !errors.Is(b.Failed, full) {
		t.Fatalf("batch = %+v, want only %v published and the publish error", b, ids[:1])
	}
	if left := h.unpublished(t); !slices.Equal(left, ids[1:]) {
		t.Fatalf("unpublished = %v, want the failed row and everything after it %v", left, ids[1:])
	}
}

func TestDrain_aCancelBetweenPublishAndMarkLeavesEveryRowUnpublished(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ids := h.appendEvents(t, 2)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var seen []uuid.UUID
	_, err := db.NewOutbox(h.pool, h.clock).Drain(ctx, 100, func(_ context.Context, row db.OutboxRow) error {
		seen = append(seen, row.ID)
		if len(seen) == 2 {
			cancel()
		}
		return nil
	})
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("drain cancelled before mark = %v, want db_unavailable", err)
	}
	if left := h.unpublished(t); !slices.Equal(left, ids) {
		t.Fatalf("unpublished = %v after a cancel before mark, want all of %v", left, ids)
	}
	if b, err := db.NewOutbox(h.pool, h.clock).Drain(t.Context(), 100, recording(&seen)); err != nil ||
		!slices.Equal(b.Published, ids) {
		t.Fatalf("redrain = %+v, %v; want both rows published", b, err)
	}
}

func TestDrain_twoDrainsSkipEachOthersLockedRows(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ids := h.appendEvents(t, 3)
	outbox := db.NewOutbox(h.pool, h.clock)
	holding, release := make(chan struct{}), make(chan struct{})
	first := make(chan db.Batch, 1)
	go func() {
		b, err := outbox.Drain(t.Context(), 2, func(context.Context, db.OutboxRow) error {
			select {
			case holding <- struct{}{}:
				<-release
			default:
			}
			return nil
		})
		if err != nil {
			t.Error(err)
		}
		first <- b
	}()
	<-holding

	second, err := outbox.Drain(t.Context(), 100, func(context.Context, db.OutboxRow) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	if got := <-first; !slices.Equal(got.Published, ids[:2]) || !slices.Equal(second.Published, ids[2:]) {
		t.Fatalf("first drain %v, second %v; want %v and %v", got.Published, second.Published, ids[:2], ids[2:])
	}
	if left := h.unpublished(t); len(left) != 0 {
		t.Fatalf("%d rows unpublished after both drains", len(left))
	}
}

func TestDrain_failsWithoutMarkingWhenThePoolIsClosed(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.appendEvents(t, 1)
	outbox := db.NewOutbox(h.pool, h.clock)
	h.pool.Close()
	b, err := outbox.Drain(t.Context(), 100, func(context.Context, db.OutboxRow) error { return nil })
	if errs.CodeOf(err) != errs.CodeInternal || len(b.Published) != 0 {
		t.Fatalf("drain on a closed pool = %+v, %v; want internal and nothing published", b, err)
	}
}

func TestBacklog_countsUnpublishedRowsAndAgesTheOldest(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	outbox := db.NewOutbox(h.pool, h.clock)
	empty, err := outbox.Backlog(t.Context())
	if err != nil || empty != (db.Backlog{}) {
		t.Fatalf("backlog of an empty outbox = %+v, %v; want zeros", empty, err)
	}
	ids := h.appendEvents(t, 2)
	h.clock.Advance(30 * time.Second)
	got, err := outbox.Backlog(t.Context())
	if err != nil || got != (db.Backlog{Unpublished: 2, Lag: 30 * time.Second}) {
		t.Fatalf("backlog = %+v, %v; want 2 rows 30s old", got, err)
	}
	if _, err := outbox.Drain(t.Context(), 1, recording(new([]uuid.UUID))); err != nil {
		t.Fatal(err)
	}
	got, err = outbox.Backlog(t.Context())
	if err != nil || got != (db.Backlog{Unpublished: 1, Lag: 30 * time.Second}) {
		t.Fatalf("backlog after publishing %v = %+v, %v; want 1 row 30s old", ids[0], got, err)
	}
	h.pool.Close()
	if _, err := outbox.Backlog(t.Context()); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("backlog on a closed pool = %v, want internal", err)
	}
}
