package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type CashOutSweeper struct {
	Payouts *app.CashOutPayouts
}

func (CashOutSweeper) Name() string { return "treasury.cashout-sweeper" }

func (CashOutSweeper) Interval() time.Duration { return app.CashOutSweepInterval }

func (s CashOutSweeper) Tick(ctx context.Context) (poller.Report, error) {
	scanned, err := s.Payouts.SweepDue(ctx)
	redriven, salesErr := s.Payouts.SweepSales(ctx)
	return poller.Report{Scanned: scanned + redriven}, errors.Join(err, salesErr)
}
