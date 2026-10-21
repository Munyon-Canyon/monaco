package funding_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
)

type refusesAtTheLock struct{ port.Balances }

func (refusesAtTheLock) AvailableIn(context.Context, dbsqlc.DBTX, port.Observation) (port.Balance, error) {
	return port.Balance{}, errs.New(errs.CodeRPCUnavailable, "test")
}

func TestWithdraw_aReadingRefusedAtTheLockMovesNoMoney(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 9_000_000, func(d *app.WithdrawDeps) { d.Balances = refusesAtTheLock{d.Balances} })
	if _, err := f.withdraw(t); errs.CodeOf(err) != errs.CodeRPCUnavailable || len(f.rows(t)) != 0 ||
		len(f.transfers.builds) != 0 {
		t.Fatalf("Handle err = %v rows = %+v builds = %d, want a retryable refusal and nothing built",
			err, f.rows(t), len(f.transfers.builds))
	}
}
