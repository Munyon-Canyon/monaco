package app

import (
	"context"
	"errors"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Resumer interface {
	Resume(ctx context.Context, id ids.SwapID) error
}

type EVMTrades struct {
	reads  sqlc.DBTX
	clock  clock.Clock
	routes Resumer
	timing SweepTiming
}

func NewEVMTrades(reads sqlc.DBTX, c clock.Clock, routes Resumer, timing SweepTiming) *EVMTrades {
	return &EVMTrades{reads: reads, clock: c, routes: routes, timing: timing}
}

func (*EVMTrades) Name() string { return "trading.evm_trades" }

func (p *EVMTrades) Interval() time.Duration { return p.timing.Interval }

func (p *EVMTrades) Tick(ctx context.Context) (poller.Report, error) {
	now := p.clock.Now()
	q := sqlc.New(p.reads)
	swaps, err := q.ListRoutedUnfinished(ctx, sqlc.ListRoutedUnfinishedParams{
		OlderThan: now.Add(-p.timing.Age), MaxRows: SwapSweepBatch,
	})
	if err != nil {
		return poller.Report{}, errs.Wrap(err, errs.CodeInternal, "trading.EVMTrades")
	}
	report := poller.Report{Scanned: len(swaps)}
	failures := make([]error, 0, len(swaps))
	for _, id := range swaps {
		_, err := q.TouchRouted(ctx, sqlc.TouchRoutedParams{ID: id, At: now})
		if err == nil {
			err = p.routes.Resume(ctx, ids.SwapIDFrom(id))
		}
		if err == nil {
			report.Changed++
		}
		failures = append(failures, err)
	}
	return report, errors.Join(failures...)
}
