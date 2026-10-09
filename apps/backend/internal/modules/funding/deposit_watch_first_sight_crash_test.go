//go:build faultpoints

package funding_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func firstSightDeposit() (time.Time, []solana.SignatureInfo) {
	created := clock.Real{}.Now().UTC().Add(-time.Hour)
	return created, []solana.SignatureInfo{{Signature: "deposit", Slot: 9, BlockTime: created.Add(time.Minute)}}
}

func (e *firstSightEnv) assertOneCandidate(t *testing.T) {
	t.Helper()
	var candidates, seen, wallets int
	if err := e.pool.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM deposit_candidates),
			(SELECT count(*) FROM events WHERE type = 'deposit.candidate_seen'),
			(SELECT count(*) FROM deposit_watch_wallets)`,
	).Scan(&candidates, &seen, &wallets); err != nil || candidates != 1 || seen != 1 || wallets != 1 {
		t.Fatalf("candidates/seen/wallets = %d/%d/%d, %v; want 1/1/1", candidates, seen, wallets, err)
	}
}

func TestFlow05_CreditDeposit_CrashFirstSightBeforeCommit(t *testing.T) {
	t.Parallel()
	created, history := firstSightDeposit()
	e := newFirstSightEnv(t, created, history)
	runs := 0
	testkit.CrashAt(t, faultpoint.FirstSightBeforeCommit, func(ctx context.Context) error {
		runs++
		if runs == 1 {
			defer func() {
				var rows int
				_ = e.pool.QueryRow(context.WithoutCancel(ctx), `SELECT
					(SELECT count(*) FROM deposit_candidates) + (SELECT count(*) FROM deposit_watch_wallets)
					+ (SELECT count(*) FROM deposit_watch_accounts)`).Scan(&rows)
				if rows != 0 {
					t.Errorf("rows after the crash = %d, want none: first sight is one transaction", rows)
				}
			}()
		} else {
			e.restart()
		}
		_, err := e.watch.Tick(observability.WithActor(ctx, "system:poller.funding.deposit_watch"))
		return err
	})
	e.assertOneCandidate(t)
	e.assertOpeningSettlesTo(t, "2")
}

func TestFlow05_CreditDeposit_CrashFirstSightAfterCommit(t *testing.T) {
	t.Parallel()
	created, history := firstSightDeposit()
	e := newFirstSightEnv(t, created, history)
	crashed := func() (r any) {
		defer func() { r = recover() }()
		ctx := faultpoint.Armed(watchActor(t), faultpoint.FirstSightAfterCommit)
		_, _ = e.watch.Tick(ctx)
		return nil
	}()
	if crashed != (faultpoint.Crash{Name: faultpoint.FirstSightAfterCommit}) {
		t.Fatalf("first run = %v, want a crash after the commit", crashed)
	}
	e.assertOneCandidate(t)
	e.restart()
	e.tick(t)
	e.assertOneCandidate(t)
	e.assertOpeningSettlesTo(t, "2")
}
