package db_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

func drainRecovering(
	t *testing.T, outbox *db.Outbox, publish func(context.Context, db.OutboxRow) error,
) (p any) {
	t.Helper()
	defer func() { p = recover() }()
	_, _ = outbox.Drain(t.Context(), 100, publish)
	return nil
}

func TestDrain_aPanickingPublishRollsBackAndUnlocksTheRowsForTheNextDrain(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ids := h.appendEvents(t, 2)
	outbox := db.NewOutbox(h.pool, h.clock)
	p := drainRecovering(t, outbox, func(_ context.Context, row db.OutboxRow) error {
		if row.ID == ids[1] {
			panic("boom")
		}
		return nil
	})
	if p != "boom" {
		t.Fatalf("Drain recovered %v, want the publish panic boom", p)
	}
	if left := h.unpublished(t); !slices.Equal(left, ids) {
		t.Fatalf("unpublished after the panic = %v, want both rows %v", left, ids)
	}
	var seen []uuid.UUID
	b, err := outbox.Drain(t.Context(), 100, recording(&seen))
	if err != nil || !slices.Equal(b.Published, ids) || !slices.Equal(seen, ids) {
		t.Fatalf("drain after the panic = %+v, %v, saw %v; want both rows published", b, err, seen)
	}
}

func TestDrain_rePanicsWithTheRollbackFailureWhenBothHappen(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.appendEvents(t, 1)
	p := drainRecovering(t, db.NewOutbox(h.pool, h.clock), func(ctx context.Context, _ db.OutboxRow) error {
		_, _ = h.pool.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity
			WHERE datname = current_database() AND pid <> pg_backend_pid() AND state = 'idle in transaction'`)
		panic("boom")
	})
	if s, _ := p.(string); !strings.HasPrefix(s, "boom (rollback: ") {
		t.Fatalf("Drain recovered %v, want boom with the rollback failure", p)
	}
}
