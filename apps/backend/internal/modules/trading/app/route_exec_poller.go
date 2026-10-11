package app

import (
	"context"
	"errors"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Resumer interface {
	Resume(ctx context.Context, id ids.SwapID) error
	BridgeBack(ctx context.Context, id ids.SwapID) error
}

type EVMTrades struct {
	reads     sqlc.DBTX
	clock     clock.Clock
	routes    Resumer
	proposals Proposals
	timing    SweepTiming
}

func NewEVMTrades(reads sqlc.DBTX, c clock.Clock, routes Resumer, proposals Proposals, timing SweepTiming) *EVMTrades {
	return &EVMTrades{reads: reads, clock: c, routes: routes, proposals: proposals, timing: timing}
}

func (*EVMTrades) Name() string { return "trading.evm_trades" }

func (p *EVMTrades) Interval() time.Duration { return p.timing.Interval }

func (p *EVMTrades) Tick(ctx context.Context) (poller.Report, error) {
	now := p.clock.Now()
	q := sqlc.New(p.reads)
	unfinished, unfinishedErr := q.ListRoutedUnfinished(ctx, sqlc.ListRoutedUnfinishedParams{
		OlderThan: now.Add(-p.timing.Age), MaxRows: SwapSweepBatch,
	})
	stranded, strandedErr := q.ListStrandedCash(ctx, sqlc.ListStrandedCashParams{
		OlderThan: now.Add(-p.timing.Age), MaxRows: SwapSweepBatch,
	})
	if err := errors.Join(unfinishedErr, strandedErr); err != nil {
		return poller.Report{}, errs.Wrap(err, errs.CodeInternal, "trading.EVMTrades")
	}
	report := poller.Report{Scanned: len(unfinished) + len(stranded)}
	failures := make([]error, 0, len(unfinished)+len(stranded))
	for _, id := range unfinished {
		_, err := q.TouchRouted(ctx, sqlc.TouchRoutedParams{ID: id, At: now})
		if err == nil {
			err = p.routes.Resume(ctx, ids.SwapIDFrom(id))
		}
		if err == nil {
			report.Changed++
		}
		failures = append(failures, err)
	}
	for _, row := range stranded {
		returned, err := p.bridgeBack(ctx, q, row, now)
		if returned && err == nil {
			report.Changed++
		}
		failures = append(failures, err)
	}
	return report, errors.Join(failures...)
}

func (p *EVMTrades) bridgeBack(
	ctx context.Context, q *sqlc.Queries, row sqlc.ListStrandedCashRow, now time.Time,
) (bool, error) {
	_, err := q.TouchFinished(ctx, sqlc.TouchFinishedParams{ID: row.ID, At: now})
	retryable := false
	if err == nil && !row.Returning && row.SourceKind == string(domain.SourceProposal) {
		retryable, err = p.proposals.Retryable(ctx, ids.ProposalIDFrom(row.SourceID))
	}
	if err != nil || retryable {
		return false, err
	}
	return true, p.routes.BridgeBack(ctx, ids.SwapIDFrom(row.ID))
}
