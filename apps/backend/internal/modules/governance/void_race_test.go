package governance_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func (d voteDB) passed(t *testing.T) uuid.UUID {
	t.Helper()
	p, _ := d.open(t, 1)
	if _, err := d.pool.Exec(t.Context(), `UPDATE proposals SET status = 'passed' WHERE id = $1`, p.ID); err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func (d voteDB) waitForLockWaiter(t *testing.T) {
	t.Helper()
	testkit.Eventually(t, func() bool {
		var waiting int
		err := d.pool.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND datname = current_database()`).
			Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		return waiting > 0
	}, 10*time.Second)
}

func TestVoidProposal_waitsForAnExecutionThatHoldsTheProposalLockAndThenSeesItsSwap(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	proposal := d.passed(t)
	swaps := fakes.NewTrading()
	locked, proceed := make(chan struct{}), make(chan struct{})
	var executing, voiding errgroup.Group
	executing.Go(func() error {
		return d.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
			wanted, err := app.NewQueries(d.pool).LockRetryable(ctx, tx, ids.ProposalIDFrom(proposal))
			if err != nil || !wanted {
				t.Errorf("LockRetryable = %t, %v, want a passed proposal to be wanted", wanted, err)
			}
			close(locked)
			<-proceed
			swaps.Put(trading.SwapView{
				ID: ids.SwapIDFrom(
					d.ids.NewV7(),
				),
				Source: trading.Source{Kind: "proposal", ID: proposal},
				Status: "created",
			})
			return err
		})
	})
	<-locked
	var voidErr error
	voiding.Go(func() error {
		voidErr = d.void(as(t, auth.ActorSystem), swaps, proposal)
		return nil
	})
	d.waitForLockWaiter(t)
	close(proceed)

	if err := executing.Wait(); err != nil {
		t.Fatal(err)
	}
	_ = voiding.Wait()
	if errs.CodeOf(voidErr) != errs.CodeLiveSwapExists {
		t.Fatalf("VoidProposal racing an execution = %v, want live_swap_exists", voidErr)
	}
	if got := d.status(t, proposal); got != "passed" {
		t.Fatalf("proposal is %s after the refused void, want passed", got)
	}
}

func TestLockRetryable_waitsForAVoidAndThenRefusesTheTrade(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	proposal := d.passed(t)
	voids := app.NewVoidProposalHandler(d.uow, d.ids, d.clk, fakes.NewTrading(), app.NoHints{})
	locked, proceed := make(chan struct{}), make(chan struct{})
	var voiding, starting errgroup.Group
	voiding.Go(func() error {
		return d.uow.Do(as(t, auth.ActorSystem), func(ctx context.Context, tx db.Tx) error {
			err := voids.Within(ctx, tx, app.VoidProposal{ProposalID: ids.ProposalIDFrom(proposal), Reason: "spam"})
			close(locked)
			<-proceed
			return err
		})
	})
	<-locked
	var wanted bool
	starting.Go(func() error {
		return d.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
			var err error
			wanted, err = app.NewQueries(d.pool).LockRetryable(ctx, tx, ids.ProposalIDFrom(proposal))
			return err
		})
	})
	d.waitForLockWaiter(t)
	close(proceed)

	if err := voiding.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := starting.Wait(); err != nil || wanted {
		t.Fatalf("LockRetryable racing a void = %t, %v, want a voided proposal to be refused", wanted, err)
	}
	if got := d.status(t, proposal); got != "voided" {
		t.Fatalf("proposal is %s, want voided", got)
	}
}

func TestLockRetryable_refusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	reads := app.NewQueries(d.pool)
	err := d.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		_, err := reads.LockRetryable(ctx, tx, ids.ProposalIDFrom(d.ids.NewV7()))
		return err
	})
	if errs.CodeOf(err) != errs.CodeProposalNotFound {
		t.Errorf("LockRetryable of an unknown proposal = %v, want proposal_not_found", err)
	}
	known := ids.ProposalIDFrom(d.passed(t))
	err = d.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		if _, err := tx.Queries().Exec(ctx, `SELECT 1/0`); err == nil {
			t.Error("division by zero did not fail")
		}
		_, err := reads.LockRetryable(ctx, tx, known)
		return err
	})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("LockRetryable in an aborted transaction = %v, want internal", err)
	}
}
