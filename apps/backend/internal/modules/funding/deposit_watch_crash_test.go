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

func TestFlow05_CreditDeposit_CrashAfterCandidate(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	ata := seedWatchAccount(t, pool, user)
	rpc := watchRPC{signatures: []solana.SignatureInfo{{Signature: "candidate", Slot: 42, BlockTime: now}}}
	p := watchFor(pool, user, now, &rpc, unlimited(), 92, 93)
	runs := 0
	testkit.CrashAt(t, faultpoint.AfterCandidate, func(ctx context.Context) error {
		runs++
		if runs == 1 {
			defer func() {
				var candidates int
				var high string
				_ = pool.QueryRow(context.WithoutCancel(ctx),
					`SELECT (SELECT count(*) FROM deposit_candidates), high_signature FROM deposit_watch_accounts`,
				).Scan(&candidates, &high)
				if candidates != 0 || high != "baseline" {
					t.Errorf("after the crash candidates = %d, high = %q; want none and an untouched cursor",
						candidates, high)
				}
			}()
		}
		_, err := p.Tick(observability.WithActor(ctx, "system:poller.funding.deposit_watch"))
		return err
	})
	var candidates, seen int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*), (SELECT count(*) FROM events WHERE type = 'deposit.candidate_seen') FROM deposit_candidates`,
	).Scan(&candidates, &seen); err != nil {
		t.Fatal(err)
	}
	if got := loadAccount(
		t,
		pool,
		ata,
	); candidates != 1 || seen != 1 || got.High != "candidate" ||
		got.CleanGen != got.DirtyGen {
		t.Fatalf(
			"after restart candidates/seen = %d/%d, account = %+v; want 1/1 and a finished scan",
			candidates,
			seen,
			got,
		)
	}
}
