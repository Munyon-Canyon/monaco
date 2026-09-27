package db

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestDo_reportsARollbackThatFailsOnADeadConnection(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	uow := New(pool, testkit.NewIDs(1), testkit.NewClock(time.Time{}))
	refused := errs.New(errs.CodeNotFound, "thing.Find")
	err := uow.Do(t.Context(), func(ctx context.Context, tx Tx) error {
		if err := tx.tx.Conn().Close(ctx); err != nil {
			return err
		}
		return refused
	})
	if !errors.Is(err, refused) || err.Error() == refused.Error() || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("Do = %v, want the closure error joined with the rollback failure", err)
	}
}

func TestDo_rePanicsWithTheRollbackFailureWhenBothHappen(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	uow := New(pool, testkit.NewIDs(1), testkit.NewClock(time.Time{}))
	defer func() {
		r, _ := recover().(string)
		if !strings.HasPrefix(r, "boom (rollback: ") {
			t.Fatalf("recovered %q, want boom with the rollback failure", r)
		}
	}()
	_ = uow.Do(t.Context(), func(ctx context.Context, tx Tx) error {
		if err := tx.tx.Conn().Close(ctx); err != nil {
			return err
		}
		panic("boom")
	})
}
