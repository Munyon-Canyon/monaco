package treasury_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestFundsAndSettlementNeverNestPoolReadsAtFourConnections(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	traced, guard := testkit.TracedPool(t, f.pool)
	f.pool, f.uow = traced, db.New(traced, f.ids, f.clock)
	fund := newFundHarnessOn(t, f, &hints{})
	fund.stubs.gate = testkit.NewGate(testkit.PressureCallers)
	h := newSettleHarnessOn(t, fund)
	landed := []uuid.UUID{h.landed(t, "1000000"), h.landed(t, "2000000")}

	ctx, cancel := context.WithTimeout(h.actorCtx(), testkit.PressureDeadline)
	defer cancel()
	probe := testkit.StartProbe(ctx, traced)
	jobs := make([]func() error, 0, testkit.PressureCallers+2)
	for range testkit.PressureCallers {
		jobs = append(jobs, func() error {
			_, err := h.handler.Handle(ctx, app.FundCabal{
				CabalID: h.cabal, UserID: h.user, Amount: money.MicrosFromUint64(1_000_000),
			})
			return err
		})
	}
	for range 2 {
		jobs = append(jobs, func() error {
			_, err := h.settler.Tick(h.systemCtx())
			return err
		})
	}
	testkit.Concurrently(t, jobs...)
	probe.Check(t)
	if ctx.Err() != nil {
		t.Fatalf("the run did not finish inside %s: %v", testkit.PressureDeadline, ctx.Err())
	}
	for range 3 {
		h.tick(t)
	}
	for _, id := range landed {
		if status, _, _ := h.status(t, id); status != "settled" {
			t.Errorf("landed transfer %s status = %s, want settled", id, status)
		}
	}
	if got := h.count(t, "fund_transfers"); got != testkit.PressureCallers+len(landed) {
		t.Errorf("fund_transfers = %d, want %d", got, testkit.PressureCallers+len(landed))
	}
	testkit.CheckNoNestedReads(t, guard)
}
