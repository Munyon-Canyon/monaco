//go:build faultpoints

package market_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestFlow18_SamplePrices_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	rig, q := newMoveRig(t, openInstant(t, clock.Real{}.Now()), aapl)
	info := equitySession(t, rig.clock.Now())
	seedPrice(t, rig.pool, aapl.Mint.String(), info.LastClose, 200_000_000)
	q.quote(aapl, 212_000_000)
	ctx := pollerCtx(t)
	armed := faultpoint.ArmedAfter(ctx, faultpoint.BeforeCommit, 1)
	crashed := func() (panicValue any) {
		defer func() { panicValue = recover() }()
		_, err := rig.poller.Tick(armed)
		if err != nil {
			t.Fatalf("tick before the crash: %v", err)
		}
		return nil
	}()
	if crashed != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
		t.Fatalf("crash = %v, want before-commit on the move", crashed)
	}
	if n := moveCount(t, rig.pool); n != 0 {
		t.Fatalf("move rows after the crash = %d, want 0", n)
	}
	if n := len(movedEvents(t, rig.pool)); n != 0 {
		t.Fatalf("events after the crash = %d, want 0", n)
	}
	if _, err := rig.poller.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	got := movedEvents(t, rig.pool)
	if len(got) != 1 {
		t.Fatalf("events after the retry = %d, want 1", len(got))
	}
	wantMove(t, got[0], aapl, 500, 600, 212_000_000, info.TradingDay.String(), rig.bucket)
}
