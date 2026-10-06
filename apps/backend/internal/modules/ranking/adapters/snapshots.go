package adapters

import (
	"context"
	"strconv"
	"time"

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
		snap, err := snapshotFrom(row)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, op)
		}
		out = append(out, snap)
	}
	return out, nil
}

func snapshotFrom(row sqlc.SnapshotsSinceRow) (domain.Snapshot, error) {
	value, err := money.SignedMicrosFromInt64(row.ValueMicros).Micros()
	if err != nil {
		return domain.Snapshot{}, err
	}
	nav, err := money.SignedMicrosFromInt64(row.NavPerShareMicros).Micros()
	if err != nil {
		return domain.Snapshot{}, err
	}
	shares, err := money.ParseSharesUnits(strconv.FormatInt(row.TotalShares, 10))
	if err != nil {
		return domain.Snapshot{}, err
	}
	return domain.Snapshot{At: row.At, Value: value, NavPerShare: nav, TotalShares: shares}, nil
}
