//go:build faultpoints

package cabal_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestCreateCabal_aCrashBeforeCommitLeavesTheWalletAndTheRetryWritesOnce(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	cmd := f.command(t, "c1")
	runs := 0
	var got app.CreatedCabal
	testkit.CrashAt(t, faultpoint.BeforeCommit, func(ctx context.Context) error {
		runs++
		if runs == 2 && (f.count(ctx, t, "cabals") != 0 || f.wallets.Creates() != 1) {
			t.Fatalf("after the crash: cabals %d creates %d; want the wallet kept and the rows rolled back",
				f.count(ctx, t, "cabals"), f.wallets.Creates())
		}
		var err error
		got, err = f.handler().Handle(f.actor(ctx), cmd)
		return err
	})
	same, err := f.wallets.CreateAppWallet(t.Context(), app.TreasuryKey(f.user.ID, "c1"))
	if err != nil || got.PrivyWalletID != same.ID || got.TreasuryAddress != same.Address {
		t.Fatalf("wallet = %+v, %v; privy = %+v", got, err, same)
	}
	cabals := f.count(t.Context(), t, "cabals")
	eventsN := f.count(t.Context(), t, "events")
	if cabals != 1 || eventsN != 2 || f.wallets.Creates() != 1 {
		t.Fatalf("cabals %d events %d creates %d; want 1, 2, 1", cabals, eventsN, f.wallets.Creates())
	}
}
