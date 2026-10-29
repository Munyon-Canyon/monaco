//go:build faultpoints

package ranking_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func TestFlow37_BuildLedgers_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	f := newLedgerFlow(t, time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC))
	week := weekOf(f.clock.Now())
	ctx := faultpoint.Armed(systemCtx(t.Context()), faultpoint.BeforeCommit)
	crashed := func() (v any) {
		defer func() { v = recover() }()
		_, _ = f.poller.Tick(ctx)
		return nil
	}()
	if crashed != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
		t.Fatalf("tick = %v, want a crash at before-commit", crashed)
	}
	if got := f.ledgers(week); got != 0 {
		t.Fatalf("ledgers after the crash = %d, want 0", got)
	}
	if got := f.readyEvents(); got != 0 {
		t.Fatalf("ledger_ready events after the crash = %d, want 0", got)
	}
	f.tick(systemCtx(t.Context()))
	if got := f.ledgers(week); got != len(f.voters) {
		t.Fatalf("ledgers after the retry = %d, want %d", got, len(f.voters))
	}
	if got := f.readyEvents(); got != len(f.voters) {
		t.Fatalf("ledger_ready events after the retry = %d, want %d", got, len(f.voters))
	}
}
