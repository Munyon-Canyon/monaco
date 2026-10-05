//go:build faultpoints

package treasury_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestFlow14_CashOut_CrashAfterSellConfirm(t *testing.T) {
	t.Parallel()
	s, r := saleAndPayout(t, 80_000_000)
	s.deliver(t, s.started)
	s.deliver(t, s.sold(s.f.ids.NewV7(), 400, 21_000_000, 2))
	last := s.sold(s.f.ids.NewV7(), 200, 10_000_000, 2)
	testkit.CrashAt(t, faultpoint.AfterSellConfirm, func(ctx context.Context) error {
		return s.applyIn(ctx, last)
	})
	s.want(t, "paying", "50000000", "0", "", 0)
	if got := r.scalar(t, `SELECT count(*)::text FROM cash_out_sells WHERE job_id = $1`, s.job); got != "2" {
		t.Fatalf("cash_out_sells rows = %s, want one per leg", got)
	}
	s.deliver(t, last)
	r.payLanded(t)
	r.wantJob(t, "completed", "")
	r.wantEnded(t, events.TypeCashOutCompleted)
	r.noDrift(t)
}
