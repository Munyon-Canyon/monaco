package adapters

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

var _ app.Snapshots = Boards{}

func (b Boards) SnapshotsSince(
	ctx context.Context, cabal ids.CabalID, since, until time.Time,
) ([]domain.Snapshot, error) {
	const op = "ranking.Boards.SnapshotsSince"
	rows, err := sqlc.New(b.DB).SnapshotsSince(ctx, sqlc.SnapshotsSinceParams{
		CabalID: cabal.UUID(), Since: since, Until: until,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	out := make([]domain.Snapshot, 0, len(rows))
	for _, row := range rows {
		snap, err := snapshotFrom(row.At, row.ValueMicros, row.NavPerShareMicros, row.TotalShares)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, op)
		}
		out = append(out, snap)
	}
	return out, nil
}

func snapshotFrom(at time.Time, value, nav, total int64) (domain.Snapshot, error) {
	v, err := money.SignedMicrosFromInt64(value).Micros()
	if err != nil {
		return domain.Snapshot{}, err
	}
	n, err := money.SignedMicrosFromInt64(nav).Micros()
	if err != nil {
		return domain.Snapshot{}, err
	}
	shares, err := money.ParseSharesUnits(strconv.FormatInt(total, 10))
	if err != nil {
		return domain.Snapshot{}, err
	}
	return domain.Snapshot{At: at, Value: v, NavPerShare: n, TotalShares: shares}, nil
}

var _ app.CabalSnapshots = Boards{}

func (b Boards) SnapshotsOfCabals(
	ctx context.Context, cabals []ids.CabalID, since, until time.Time,
) (map[ids.CabalID][]domain.Snapshot, error) {
	const op = "ranking.Boards.SnapshotsOfCabals"
	keys := make([]uuid.UUID, 0, len(cabals))
	for _, c := range cabals {
		keys = append(keys, c.UUID())
	}
	rows, err := sqlc.New(b.DB).SnapshotsOfCabals(ctx, sqlc.SnapshotsOfCabalsParams{
		CabalIds: keys, Since: since, Until: until,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	out := make(map[ids.CabalID][]domain.Snapshot, len(cabals))
	for _, row := range rows {
		snap, err := snapshotFrom(row.At, row.ValueMicros, row.NavPerShareMicros, row.TotalShares)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, op)
		}
		cabal := ids.CabalIDFrom(row.CabalID)
		out[cabal] = append(out[cabal], snap)
	}
	return out, nil
}

var _ app.LatestSnapshots = Boards{}

func (b Boards) LatestValuesOf(ctx context.Context, cabals []ids.CabalID) (map[ids.CabalID]domain.Snapshot, error) {
	const op = "ranking.Boards.LatestValuesOf"
	keys := make([]uuid.UUID, 0, len(cabals))
	for _, c := range cabals {
		keys = append(keys, c.UUID())
	}
	rows, err := sqlc.New(b.DB).LatestValuesOfCabals(ctx, keys)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	out := make(map[ids.CabalID]domain.Snapshot, len(rows))
	for _, row := range rows {
		snap, err := snapshotFrom(row.At, row.ValueMicros, row.NavPerShareMicros, row.TotalShares)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, op)
		}
		out[ids.CabalIDFrom(row.CabalID)] = snap
	}
	return out, nil
}
