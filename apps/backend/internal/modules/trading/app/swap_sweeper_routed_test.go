package app

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func (e *sweeperEnv) route(t *testing.T, swap uuid.UUID) {
	t.Helper()
	if _, err := e.pool.Exec(t.Context(), `INSERT INTO trade_steps
		(id, trade_id, step, network, amount_micros, status, created_at, updated_at)
		VALUES ($1, $2, 'bridge_out', 'base', 25000000, 'pending', $3, $3)`,
		e.ids.NewV7(), swap, e.clk.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestSwapSweeper_leavesRoutedSwapsToTheirSteps(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	created, submitted, plain := e.insert(t), e.submit(t), e.insert(t)
	e.route(t, created.ID)
	e.route(t, submitted.ID)
	e.advance()

	report, err := e.poller().Tick(tickContext(t))
	if err != nil || report.Scanned != 1 || report.Changed != 1 {
		t.Fatalf("Tick = %+v, %v, want only the unrouted swap scanned and failed", report, err)
	}
	e.assertStates(t, []sweeperState{
		{id: created.ID, status: string(domain.StatusCreated)},
		{id: submitted.ID, status: string(domain.StatusSubmitted)},
		{id: plain.ID, status: string(domain.StatusFailed), events: 1},
	})
}

func TestForceResolve_refusesARoutedSwap(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row := e.submit(t)
	e.route(t, row.ID)

	_, err := e.force().Handle(t.Context(), forceCommand(chain.Signature("sig-"+row.ID.String()), domain.StatusFailed))
	if errs.CodeOf(err) != errs.CodeSwapNotStuck {
		t.Fatalf("Handle = %v, want swap_not_stuck", err)
	}
	if got := e.status(t, row.ID); got != string(domain.StatusSubmitted) || e.terminalEvents(t, row.ID) != 0 {
		t.Fatalf("swap = %s with %d terminal events, want submitted and untouched", got, e.terminalEvents(t, row.ID))
	}
}
