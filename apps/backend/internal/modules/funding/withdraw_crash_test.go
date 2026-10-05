//go:build faultpoints

package funding_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func TestFlow15_Withdraw_CrashAfterSign(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	crashed := func() (p any) {
		defer func() { p = recover() }()
		_, _ = f.handle(faultpoint.Armed(f.ctx(t.Context()), faultpoint.AfterSign), withdrawTo)
		return nil
	}()
	if crashed != (faultpoint.Crash{Name: faultpoint.AfterSign}) {
		t.Fatalf("Handle did not crash at after-sign: %v", crashed)
	}
	if rows := f.rows(t); len(rows) != 1 || rows[0].status != "created" || len(f.transfers.sent) != 0 {
		t.Fatalf("after the crash rows = %+v sent = %d", rows, len(f.transfers.sent))
	}
	f.assertInFlight(t, "2000000")
	p := f.poller(status(solana.StateNotFound, false, 0))
	f.clock.Advance(app.WithdrawalUnsentAge + time.Second)
	if changed, err := f.tick(t, p); err != nil || changed != 1 {
		t.Fatalf("Tick after 2 min = %d, %v", changed, err)
	}
	f.assertNotSent(t)
	if len(f.transfers.sent) != 0 {
		t.Fatalf("sent = %d after the crash", len(f.transfers.sent))
	}
}
