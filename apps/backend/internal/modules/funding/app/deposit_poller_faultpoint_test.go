//go:build faultpoints

package app

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestFlow05_CreditDeposit_CrashAfterCandidate(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO deposit_cursors (wallet_address, last_signature, cursor_slot, scanned_at)
		VALUES ($1, 'baseline', 1, $2)`, user.Address, now,
	); err != nil {
		t.Fatal(err)
	}
	p := DepositPoller{
		uow:   db.New(pool, testkit.NewIDs(92), testkit.NewClock(now)),
		ids:   testkit.NewIDs(93),
		clock: testkit.NewClock(now),
	}
	wallet := port.MemberWallet{UserID: user.ID, Address: user.Address}
	page := []solana.SignatureInfo{{Signature: "candidate", Slot: 42, BlockTime: now}}
	testkit.CrashAt(t, faultpoint.AfterCandidate, func(ctx context.Context) error {
		_, err := p.processBackfillPage(ctx, wallet, page, &backfillCursor{})
		return err
	})
	var candidates int
	var cursor string
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*), (SELECT last_signature FROM deposit_cursors WHERE wallet_address = $1)
		FROM deposit_candidates WHERE wallet_address = $1`, user.Address,
	).Scan(&candidates, &cursor); err != nil {
		t.Fatal(err)
	}
	if candidates != 1 || cursor != "candidate" {
		t.Fatalf("candidate/checkpoint after restart = %d/%q, want 1/%q", candidates, cursor, "candidate")
	}
}
