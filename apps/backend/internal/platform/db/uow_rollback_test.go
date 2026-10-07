package db_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
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

func TestDo_keepsTheClosureCodeWhenTheRollbackFailsTransiently(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	uow := db.New(pool, testkit.NewIDs(1), testkit.NewClock(time.Time{}))
	invariant := errs.New(errs.CodeInternal, "withdrawal.Submit")
	err := uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		killConnection(ctx, tx)
		return invariant
	})
	if got := errs.CodeOf(err); got != errs.CodeInternal {
		t.Fatalf("Do code = %s (%v), want internal kept despite the rollback failure", got, err)
	}
	if !errors.Is(err, invariant) || !errors.Is(err, pgconn.ErrConnClosed) {
		t.Fatalf("Do = %v, want the closure error joined with the rollback failure", err)
	}
}
