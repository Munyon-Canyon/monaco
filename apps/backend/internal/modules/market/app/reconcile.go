package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const ReconcileInterval = 24 * time.Hour

type Reconcile struct {
	uow     *db.UnitOfWork
	catalog *Catalog
	history PriceHistory
}

func NewReconcile(uow *db.UnitOfWork, reads sqlc.DBTX, history PriceHistory) *Reconcile {
	return &Reconcile{uow: uow, catalog: NewCatalog(reads), history: history}
}

func (*Reconcile) Name() string { return "market.reconcile" }

func (*Reconcile) Interval() time.Duration { return ReconcileInterval }

func (r *Reconcile) Tick(ctx context.Context) (poller.Report, error) {
	if !r.history.Configured() {
		observability.Degraded(ctx, observability.MarketReconcileSkippedNoKey)
		return poller.Report{}, nil
	}
	assets, err := r.catalog.ListPriceable(ctx)
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
