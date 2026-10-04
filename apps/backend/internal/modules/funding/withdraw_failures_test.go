package funding_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const refuseWithdrawalWrites = `
CREATE FUNCTION refuse_withdrawal_write() RETURNS trigger LANGUAGE plpgsql AS
$$ BEGIN RAISE EXCEPTION 'refused'; END $$;
CREATE TRIGGER refuse_withdrawal_write BEFORE %s ON withdrawals
FOR EACH ROW EXECUTE FUNCTION refuse_withdrawal_write();`

func TestWithdraw_WriteFailuresSendNothing(t *testing.T) {
	t.Parallel()
	for _, op := range []string{"INSERT", "UPDATE"} {
		t.Run(op, func(t *testing.T) {
			t.Parallel()
			f := newWithdrawFixture(t, 5_000_000)
			if _, err := f.pool.Exec(t.Context(), fmt.Sprintf(refuseWithdrawalWrites, op)); err != nil {
				t.Fatal(err)
			}
			if _, err := f.withdraw(t); err == nil || len(f.transfers.sent) != 0 {
				t.Fatalf("Handle err = %v sent = %d", err, len(f.transfers.sent))
			}
		})
	}
}

func TestWithdraw_SubmitWithoutActorSendsNothing(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	_, err := f.handle(t.Context(), withdrawTo)
	if err == nil || len(f.transfers.sent) != 0 || len(f.submittedEvents(t)) != 0 {
		t.Fatalf("Handle err = %v sent = %d", err, len(f.transfers.sent))
	}
}

func TestWithdraw_WaitsOnTheOutflowLock(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	holder, err := f.pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	defer func() { _, _ = holder.Exec(context.Background(), `SELECT pg_advisory_unlock_all()`) }()
	if _, err := holder.Exec(t.Context(),
		`SELECT pg_advisory_lock(hashtextextended('wallet-outflow:' || $1::uuid::text, 0))`, f.user.ID.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(f.ctx(t.Context()))
	defer cancel()
	done := make(chan error, 1)
	var handler sync.WaitGroup
	t.Cleanup(handler.Wait)
	handler.Go(func() { _, err := f.handle(ctx, withdrawTo); done <- err })
	testkit.Eventually(t, func() bool {
		var waiting int
		err := f.pool.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted
			AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`).Scan(&waiting)
		return err == nil && waiting == 1
	}, 10*time.Second)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) || len(f.rows(t)) != 0 {
		t.Fatalf("Handle err = %v rows = %+v", err, f.rows(t))
	}
}
