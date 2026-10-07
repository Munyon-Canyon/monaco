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

const (
	BackfillInterval = 5 * time.Minute
	backfillBatch    = 10
)

type historyWindow struct {
	days   int
	bucket time.Duration
}

func finestFirstWindows() []historyWindow {
	return []historyWindow{
		{days: 1, bucket: 5 * time.Minute},
		{days: 90, bucket: time.Hour},
		{days: 365, bucket: 24 * time.Hour},
	}
}

type Backfill struct {
	uow     *db.UnitOfWork
	reads   sqlc.DBTX
	clock   clock.Clock
	history PriceHistory
}

func NewBackfill(uow *db.UnitOfWork, reads sqlc.DBTX, c clock.Clock, history PriceHistory) *Backfill {
	return &Backfill{uow: uow, reads: reads, clock: c, history: history}
}

func (*Backfill) Name() string { return "market.backfill" }

func (*Backfill) Interval() time.Duration { return BackfillInterval }

type BackfillResult struct {
	Mints, Calls, Rows int
}

func (b *Backfill) Tick(ctx context.Context) (poller.Report, error) {
	if !b.history.Configured() {
		observability.Degraded(ctx, observability.MarketBackfillSkippedNoKey)
		return poller.Report{}, nil
	}
	pending, err := sqlc.New(b.reads).PendingBackfills(ctx,
		sqlc.PendingBackfillsParams{Now: b.clock.Now(), BatchLimit: backfillBatch})
	if err != nil {
		return poller.Report{}, errs.Wrap(err, errs.CodeOf(err), "market.Backfill.Tick")
	}
	res, err := b.Drain(ctx, pending)
	return poller.Report{
		Scanned: res.Mints, Changed: res.Rows,
		Attrs: []slog.Attr{slog.Int("calls", res.Calls), slog.Int("pending", len(pending)-res.Mints)},
	}, err
}

func (b *Backfill) Run(ctx context.Context, mints []string) (BackfillResult, error) {
	err := b.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := sqlc.New(tx.Queries()).RequestBackfills(ctx,
			sqlc.RequestBackfillsParams{Mints: mints, Now: b.clock.Now()})
		return err
	})
	if err != nil {
		return BackfillResult{}, errs.Wrap(err, errs.CodeOf(err), "market.Backfill.Run", slog.Int("mints", len(mints)))
	}
	return b.Drain(ctx, mints)
}

func (b *Backfill) RunAll(ctx context.Context) (BackfillResult, error) {
	assets, err := NewCatalog(b.reads).ListPriceable(ctx)
	if err != nil {
		return BackfillResult{}, err
	}
	mints := make([]string, len(assets))
	for i, a := range assets {
		mints[i] = a.Mint.String()
	}
	return b.Run(ctx, mints)
}

func (b *Backfill) Drain(ctx context.Context, mints []string) (BackfillResult, error) {
	var res BackfillResult
	var failed []error
	for _, raw := range mints {
		rows, calls, err := b.backfill(ctx, raw)
		res.Calls += calls
		res.Rows += rows
		res.Mints++
		if err != nil {
			failed = append(failed, err)
		}
		if errs.CodeOf(err) == errs.CodeCoinGeckoRateLimited {
			break
		}
	}
	return res, errors.Join(failed...)
}

func (b *Backfill) backfill(ctx context.Context, raw string) (int, int, error) {
	const op = "market.Backfill.backfill"
	mint, err := domain.ParseMint(raw)
	if err != nil {
		return 0, 0, b.fail(ctx, raw, errs.Wrap(err, errs.CodeInvalidAddress, op, slog.String("mint", raw)))
	}
	answers := make([][]Sample, 0, len(finestFirstWindows()))
	calls := 0
	for _, w := range finestFirstWindows() {
		calls++
		samples, err := b.history.MarketChart(ctx, mint, w.days)
		if err != nil {
			return 0, calls, b.fail(ctx, raw, errs.Wrap(err, errs.CodeOf(err), op, slog.Int("days", w.days)))
		}
		answers = append(answers, samples)
	}
	var rows int64
	err = b.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		rows = 0
		for i, w := range finestFirstWindows() {
			n, err := q.InsertBackfilledPricePoints(ctx, insertParams(mint, w, answers[i]))
			if err != nil {
				return err
			}
			rows += n
		}
		return q.FinishBackfill(ctx, sqlc.FinishBackfillParams{Mint: raw, Now: b.clock.Now()})
	})
	if err != nil {
		return 0, calls, b.fail(ctx, raw, errs.Wrap(err, errs.CodeOf(err), op, slog.String("mint", raw)))
	}
	return int(rows), calls, nil
}

func insertParams(mint domain.Mint, w historyWindow, samples []Sample) sqlc.InsertBackfilledPricePointsParams {
	p := sqlc.InsertBackfilledPricePointsParams{Mint: mint.String(), Source: string(domain.SourceCoinGecko)}
	for _, s := range samples {
		if stored, ok := storableMicros(s.Price); ok {
			p.Timestamps = append(p.Timestamps, s.At.Truncate(w.bucket))
			p.PriceMicros = append(p.PriceMicros, stored)
		}
	}
	return p
}

func (b *Backfill) fail(ctx context.Context, mint string, cause error) error {
	err := b.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return sqlc.New(tx.Queries()).FailBackfill(ctx,
			sqlc.FailBackfillParams{
				Mint: mint, Code: string(errs.CodeOf(cause)), Now: b.clock.Now(),
				BackOff: errs.CodeOf(cause) != errs.CodeCoinGeckoRateLimited,
			})
	})
	return errors.Join(cause, err)
}
