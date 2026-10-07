package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const ReconcileInterval = 24 * time.Hour

const reconcileColdPerTick = 150

type Reconcile struct {
	uow     *db.UnitOfWork
	catalog *Catalog
	history PriceHistory
	clock   clock.Clock
	hot     []HotMints
}

func NewReconcile(
	uow *db.UnitOfWork, reads sqlc.DBTX, c clock.Clock, history PriceHistory, hot ...HotMints,
) *Reconcile {
	return &Reconcile{uow: uow, catalog: NewCatalog(reads), history: history, clock: c, hot: hot}
}

func (*Reconcile) Name() string { return "market.reconcile" }

func (*Reconcile) Interval() time.Duration { return ReconcileInterval }

func (r *Reconcile) Tick(ctx context.Context) (poller.Report, error) {
	if !r.history.Configured() {
		observability.Degraded(ctx, observability.MarketReconcileSkippedNoKey)
		return poller.Report{}, nil
	}
	assets, err := r.nightly(ctx)
	if err != nil {
		return poller.Report{}, err
	}
	var report poller.Report
	var failed []error
	for _, a := range assets {
		rows, err := r.fill(ctx, a.Mint)
		report.Scanned++
		report.Changed += rows
		if err != nil {
			failed = append(failed, err)
		}
		if errs.CodeOf(err) == errs.CodeCoinGeckoRateLimited {
			break
		}
	}
	report.Attrs = []slog.Attr{slog.Int("calls", report.Scanned), slog.Int("failed", len(failed))}
	return report, errors.Join(failed...)
}

func (r *Reconcile) nightly(ctx context.Context) ([]domain.Asset, error) {
	all, err := r.catalog.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	hot, err := hotSet(ctx, r.hot, all)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.Reconcile.nightly")
	}
	var out, cold []domain.Asset
	for _, a := range all {
		switch {
		case hot[a.Mint.Address()]:
			out = append(out, a)
		case a.Tradable():
			cold = append(cold, a)
		}
	}
	return append(out, coldSlot(cold, r.clock.Now(), ReconcileInterval, reconcileColdPerTick)...), nil
}

func (r *Reconcile) fill(ctx context.Context, mint domain.Mint) (int, error) {
	const op = "market.Reconcile.fill"
	window := historyWindow{days: 2, bucket: time.Hour}
	samples, err := r.history.MarketChart(ctx, mint, window.days)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), op, slog.String("mint", mint.String()))
	}
	var rows int64
	err = r.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		rows, err = sqlc.New(tx.Queries()).InsertBackfilledPricePoints(ctx, insertParams(mint, window, samples))
		return err
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), op, slog.String("mint", mint.String()))
	}
	return int(rows), nil
}
