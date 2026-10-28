package governance_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type pooledCabals struct {
	app.Cabals
	reads *pgxpool.Pool
	gate  *testkit.Gate
}

func (c pooledCabals) Status(ctx context.Context, _ ids.CabalID) (cabalport.Status, error) {
	c.gate.Arrive()
	_, err := c.reads.Exec(ctx, `SELECT 1`)
	return cabalport.StatusActive, err
}

func TestVotesNeverNestPoolReadsAtFourConnections(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	pool, guard := testkit.TracedPool(t, d.pool)
	uow := db.New(pool, d.ids, d.clk)
	gate := testkit.NewGate(testkit.PressureCallers)
	handler := app.NewCastVoteHandler(uow, pooledCabals{reads: pool, gate: gate}, d.clk, app.NoHints{})
	voters := []uuid.UUID{d.ids.NewV7(), d.ids.NewV7(), d.ids.NewV7()}
	proposals := make([]uuid.UUID, 2)
	for i := range proposals {
		p := d.buy(voters...)
		d.insert(t, p)
		proposals[i] = p.ID
	}

	ctx, cancel := testkit.PressureContext(t)
	defer cancel()
	probe := testkit.StartProbe(ctx, pool)
	jobs := make([]func() error, 0, len(voters)*len(proposals))
	for _, proposal := range proposals {
		for _, voter := range voters {
			jobs = append(jobs, func() error {
				_, err := d.cast(ctx, handler, proposal, voter, domain.ChoiceYes)
				if errs.CodeOf(err) == errs.CodeProposalClosed {
					return nil
				}
				return err
			})
		}
	}
	testkit.Concurrently(t, jobs...)
	probe.Check(t)
	if ctx.Err() != nil {
		t.Fatalf("the run did not finish before the test deadline: %v", ctx.Err())
	}
	for _, proposal := range proposals {
		if d.terminal(t, proposal) != 1 {
			t.Errorf("proposal %s terminal events = %d, want exactly one", proposal, d.terminal(t, proposal))
		}
	}
	testkit.CheckNoNestedReads(t, guard)
}
