//go:build faultpoints

package db_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func TestDo_crashBeforeCommitRollsBackAndRePanicsTheCrash(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := faultpoint.Armed(h.ctx(t, "user:u1"), faultpoint.BeforeCommit)
	defer func() {
		if r := recover(); r != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
			t.Fatalf("recovered %v, want Crash{before-commit}", r)
		}
		if h.count(t, "things") != 0 || h.count(t, "events") != 0 || h.pendingSignals() != 0 {
			t.Fatalf("things=%d events=%d signals=%d after a crash before commit, want zeros",
				h.count(t, "things"), h.count(t, "events"), h.pendingSignals())
		}
		h.assertOneLine(t, "tx.rolled_back", map[string]any{"code": "panic", "attempt": float64(1)})
	}()
	_ = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		if err := h.insertThing(ctx, tx, 1); err != nil {
			return err
		}
		return tx.Events.Append(ctx, pinged(h.ids))
	})
}
