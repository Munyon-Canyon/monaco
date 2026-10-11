//go:build faultpoints

package funding_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func TestWithdrawEVM_CrashAfterCreateNeverSendsASecondTransfer(t *testing.T) {
	t.Parallel()
	actions := &privyActions{}
	f := newEVMFixture(t, actions)
	crashed := func() (p any) {
		defer func() { p = recover() }()
		_, _ = f.withdrawEVM(faultpoint.Armed(f.ctx(t.Context()), faultpoint.AfterCreate))
		return nil
	}()
	if crashed != (faultpoint.Crash{Name: faultpoint.AfterCreate}) {
		t.Fatalf("Handle did not crash at after-create: %v", crashed)
	}
	if row := f.evmRow(t); row.status != "created" || row.action != "" || len(actions.calls) != 1 {
		t.Fatalf("after the crash row = %+v transfers = %d", row, len(actions.calls))
	}
	f.assertInFlight(t, "2000000")
	f.clock.Advance(withdrawalUnsentAge + time.Second)
	if changed, err := f.tick(t, f.evmPoller(actions)); err != nil || changed != 1 {
		t.Fatalf("Tick = %d, %v", changed, err)
	}
	assertOneAction(t, f, actions)
	f.assertInFlight(t, "2000000")
}
