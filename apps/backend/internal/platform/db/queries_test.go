package db_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestQueries_uowDoWithOneWriteAndOneEvent(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.ctx(t, "user:u1")
	testkit.AssertQueries(t, "uow.Do one insert and one event", func() {
		err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
			if err := h.insertThing(ctx, tx, 1); err != nil {
				return err
			}
			return tx.Events.Append(ctx, pinged(h.ids))
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}
