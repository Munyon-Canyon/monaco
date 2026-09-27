package db_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestDo_reportsARollbackThatFailsOnADeadConnection(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	uow := db.New(pool, testkit.NewIDs(1), testkit.NewClock(time.Time{}))
	refused := errs.New(errs.CodeNotFound, "thing.Find")
	err := uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		killConnection(ctx, tx)
		return refused
	})
	if !errors.Is(err, refused) || err.Error() == refused.Error() || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("Do = %v, want the closure error joined with the rollback failure", err)
	}
}

func killConnection(ctx context.Context, tx db.Tx) {
	_, _ = tx.Queries().Exec(ctx, `SELECT pg_terminate_backend(pg_backend_pid())`)
}

func TestDo_rePanicsWithTheRollbackFailureWhenBothHappen(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	uow := db.New(pool, testkit.NewIDs(1), testkit.NewClock(time.Time{}))
	defer func() {
		r, _ := recover().(string)
		if !strings.HasPrefix(r, "boom (rollback: ") {
			t.Fatalf("recovered %q, want boom with the rollback failure", r)
		}
	}()
	_ = uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		killConnection(ctx, tx)
		panic("boom")
	})
}

func TestDo_rePanicsACrashUnchangedWhenTheRollbackFails(t *testing.T) {
	t.Parallel()
	uow := db.New(testkit.DB(t), testkit.NewIDs(1), testkit.NewClock(time.Time{}))
	crash := faultpoint.Crash{Name: faultpoint.AfterExecute}
	defer func() {
		if r := recover(); r != crash {
			t.Fatalf("recovered %v, want the Crash itself", r)
		}
	}()
	_ = uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		killConnection(ctx, tx)
		panic(crash)
	})
}
