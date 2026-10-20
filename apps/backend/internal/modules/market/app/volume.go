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
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const VolumeInterval = time.Hour

const VolumeBatch = 250

type VolumeSource interface {
	Configured() bool
	Volumes(ctx context.Context, mints []domain.Mint) (map[domain.Mint]money.Micros, error)
}

type Volumes struct {
	uow     *db.UnitOfWork
	catalog *Catalog
	source  VolumeSource
	clock   clock.Clock
	cool    *Cooldown
}

func NewVolumes(uow *db.UnitOfWork, reads sqlc.DBTX, c clock.Clock, source VolumeSource) *Volumes {
	return &Volumes{uow: uow, catalog: NewCatalog(reads), source: source, clock: c, cool: NewCooldown(c)}
}

func (v *Volumes) WithCooldown(c *Cooldown) *Volumes {
	v.cool = c
	return v
}

func (*Volumes) Name() string { return "market.volume" }

func (*Volumes) Interval() time.Duration { return VolumeInterval }

func (v *Volumes) Tick(ctx context.Context) (poller.Report, error) {
	if !v.source.Configured() {
		return poller.Report{}, nil
	}
	all, err := v.catalog.ListAll(ctx)
	if err != nil {
		return poller.Report{}, err
	}
	var mints []domain.Mint
	for _, a := range all {
		if a.Tradable() {
			mints = append(mints, a.Mint)
		}
	}
	var report poller.Report
	var failed []error
	calls := 0
	for len(mints) > 0 {
		if v.cool.Active() {
			break
		}
		n := min(len(mints), VolumeBatch)
		batch := mints[:n]
		mints = mints[n:]
		calls++
		report.Scanned += n
		rows, err := v.fill(ctx, batch)
		report.Changed += rows
		if err != nil {
			failed = append(failed, err)
		}
		v.cool.Trip(ctx, v.Name(), err)
	}
	report.Attrs = []slog.Attr{
		slog.Int("calls", calls), slog.Int("failed", len(failed)), slog.Int("skipped", len(mints)),
	}
	return report, errors.Join(failed...)
}

func (v *Volumes) fill(ctx context.Context, mints []domain.Mint) (int, error) {
	const op = "market.Volumes.fill"
	got, err := v.source.Volumes(ctx, mints)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), op, slog.Int("mints", len(mints)))
	}
	now := v.clock.Now()
	written := 0
	err = v.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		for _, m := range mints {
			usd, ok := got[m]
			if !ok {
				continue
			}
			if err := q.UpsertAssetVolume(ctx, volumeParams(m, Volume{At: now, USD: usd}, now)); err != nil {
				return err
			}
			written++
		}
		return nil
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), op, slog.Int("mints", len(mints)))
	}
	return written, nil
}
