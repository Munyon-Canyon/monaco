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
	cool    *Cooldown
	budget  *Budget
}

func NewReconcile(
	uow *db.UnitOfWork, reads sqlc.DBTX, c clock.Clock, history PriceHistory, hot ...HotMints,
) *Reconcile {
	return &Reconcile{uow: uow, catalog: NewCatalog(reads), history: history, clock: c, hot: hot, cool: NewCooldown(c)}
}

func (r *Reconcile) WithCooldown(c *Cooldown) *Reconcile {
	r.cool = c
	return r
}

func (r *Reconcile) WithBudget(b *Budget) *Reconcile {
	r.budget = b
	return r
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
	skipped := 0
	for _, a := range assets {
		if r.overBudget(ctx, a.cold) {
			skipped++
			continue
		}
		if err := r.cool.Wait(ctx); err != nil {
			failed = append(failed, err)
			break
		}
		rows, err := r.fill(ctx, a.asset.Mint)
		report.Scanned++
		report.Changed += rows
		if err != nil {
			failed = append(failed, err)
		}
		r.cool.Trip(ctx, r.Name(), err)
	}
	report.Attrs = []slog.Attr{
		slog.Int("calls", report.Scanned), slog.Int("failed", len(failed)), slog.Int("skipped", skipped),
	}
	return report, errors.Join(failed...)
}

func (r *Reconcile) overBudget(ctx context.Context, cold bool) bool {
	level := r.budget.Level(ctx)
	return level == BudgetHard || (level == BudgetSoft && cold)
}

type reconcileAsset struct {
	asset domain.Asset
	cold  bool
}

func (r *Reconcile) nightly(ctx context.Context) ([]reconcileAsset, error) {
	all, err := r.catalog.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	hot, err := hotSet(ctx, r.hot, all)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.Reconcile.nightly")
	}
	var out []reconcileAsset
	var cold []domain.Asset
	for _, a := range all {
		switch {
		case hot[a.Mint.Address()]:
			out = append(out, reconcileAsset{asset: a})
		case a.Tradable():
			cold = append(cold, a)
		}
	}
	for _, a := range coldSlot(cold, r.clock.Now(), ReconcileInterval, reconcileColdPerTick) {
		out = append(out, reconcileAsset{asset: a, cold: true})
	}
	return out, nil
}

func (r *Reconcile) fill(ctx context.Context, mint domain.Mint) (int, error) {
	const op = "market.Reconcile.fill"
	window := historyWindow{days: 2, bucket: time.Hour}
	history, err := r.history.MarketChart(ctx, mint, window.days)
	if errs.CodeOf(err) == errs.CodeNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), op, slog.String("mint", mint.String()))
	}
	var rows int64
	err = r.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		var err error
		rows, err = q.InsertReconciledPricePoints(ctx, insertParams(mint, window, history.Prices))
		if err != nil || history.Volume24h == nil {
			return err
		}
		return q.UpsertAssetVolume(ctx, volumeParams(mint, *history.Volume24h, r.clock.Now()))
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), op, slog.String("mint", mint.String()))
	}
	return int(rows), nil
}

func volumeParams(mint domain.Mint, v Volume, now time.Time) sqlc.UpsertAssetVolumeParams {
	micros, _ := storableMicros(v.USD)
	return sqlc.UpsertAssetVolumeParams{Mint: mint.String(), Volume24hMicros: micros, AsOf: v.At, UpdatedAt: now}
}

type historyWindow struct {
	days   int
	bucket time.Duration
}

func insertParams(mint domain.Mint, w historyWindow, samples []Sample) sqlc.InsertReconciledPricePointsParams {
	p := sqlc.InsertReconciledPricePointsParams{Mint: mint.String(), Source: string(domain.SourceCoinGecko)}
	for _, s := range samples {
		if stored, ok := storableMicros(s.Price); ok {
			p.Timestamps = append(p.Timestamps, s.At.Truncate(w.bucket))
			p.PriceMicros = append(p.PriceMicros, stored)
		}
	}
	return p
}
