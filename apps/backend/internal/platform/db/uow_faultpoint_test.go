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
		h.assertOneLine(t, "tx.crashed", map[string]any{"code": "faultpoint", "attempt": float64(1)})
	}()
	_ = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		if err := h.insertThing(ctx, tx, 1); err != nil {
			return err
		}
		return tx.Events.Append(ctx, pinged(h.ids))
	})
}

func TestDo_armedAfterTwoCommitsTwiceThenCrashesTheThird(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := faultpoint.ArmedAfter(h.ctx(t, "user:u1"), faultpoint.BeforeCommit, 2)
	do := func(id int) (p any) {
		defer func() { p = recover() }()
		_ = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error { return h.insertThing(ctx, tx, id) })
		return nil
	}
	got := []any{do(1), do(2), do(3)}
	if got[0] != nil || got[1] != nil || got[2] != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
		t.Fatalf("three Do calls recovered %v, want nil, nil, Crash{before-commit}", got)
	}
	if n := h.count(t, "things"); n != 2 {
		t.Fatalf("things = %d, want the first two committed", n)
	}
}

func TestDo_crashBeforeCommitSurvivesAFailedRollback(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	defer func() {
		if r := recover(); r != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
			t.Fatalf("recovered %v, want Crash{before-commit}", r)
		}
	}()
	_ = h.uow.Do(faultpoint.Armed(h.ctx(t, "user:u1"), faultpoint.BeforeCommit),
		func(ctx context.Context, tx db.Tx) error {
			killConnection(ctx, tx)
			return nil
		})
}
