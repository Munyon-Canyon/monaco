package cabal_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestLockStatus_readsTheStatusAndHoldsOffABanUntilTheTransactionEnds(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	q := adapters.NewQueries(f.pool)
	ban := `UPDATE cabals SET status = 'banned' WHERE id = $1 AND status = 'active'`
	err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		if got, err := q.LockStatus(ctx, tx.Queries(), c.ID); err != nil || got != port.StatusActive {
			t.Errorf("LockStatus = %q, %v; want active", got, err)
		}
		blocked, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		defer cancel()
		if _, err := f.pool.Exec(blocked, ban, c.ID.UUID()); err == nil {
			t.Error("a ban ran while the status was locked, want it to wait")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), ban, c.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	err = f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		if got, err := q.LockStatus(ctx, tx.Queries(), c.ID); err != nil || got != port.StatusBanned {
			t.Errorf("LockStatus after the ban = %q, %v; want banned", got, err)
		}
		_, err := q.LockStatus(ctx, tx.Queries(), ids.CabalIDFrom(ids.Real{}.NewV7()))
		wantErr(t, err, errs.CodeCabalNotFound)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLockStatus_wrapsABrokenRead(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool, testkit.WithMembers(2))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := adapters.NewQueries(pool).LockStatus(ctx, pool, c.ID)
	wantErr(t, err, errs.CodeInternal)
}
