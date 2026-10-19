package adapters

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Latest struct{ DB sqlc.DBTX }

func (l Latest) LatestRun(ctx context.Context) (port.Run, error) {
	row, err := sqlc.New(l.DB).LastLeaderboardRun(ctx)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return port.Run{}, errs.New(errs.CodeNotFound, "ranking.Latest.LatestRun")
	case err != nil:
		return port.Run{}, errs.Wrap(err, errs.CodeOf(err), "ranking.Latest.LatestRun")
	}
	return port.Run{
		RunID: row.RunID, AsOf: row.AsOf, PricesAsOf: row.PricesAsOf, StartedAt: row.StartedAt,
		FinishedAt: row.FinishedAt, RowsWritten: int(row.RowsWritten), CabalsExcluded: int(row.CabalsExcluded),
		Rev: int(row.Rev),
	}, nil
}

func (l Latest) LatestCabalValues(ctx context.Context) ([]port.CabalValue, error) {
	rows, err := sqlc.New(l.DB).LatestCabalValues(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "ranking.Latest.LatestCabalValues")
	}
	out := make([]port.CabalValue, len(rows))
	for i, row := range rows {
		value, perShare, shares := row.ValueMicros, row.NavPerShareMicros, row.TotalShares
		if value < 0 || perShare < 0 || shares < 0 {
			return nil, errs.New(errs.CodeDecodeFailed, "ranking.Latest.LatestCabalValues")
		}
		out[i] = port.CabalValue{
			CabalID: ids.CabalIDFrom(row.CabalID), At: row.At, Value: money.MicrosFromUint64(uint64(value)),
			NavPerShare: money.MicrosFromUint64(uint64(perShare)),
			TotalShares: money.SharesUnitsFromUint64(uint64(shares)),
		}
	}
	return out, nil
}

func (l Latest) PeopleReturns(
	ctx context.Context, rng domain.Range, users []ids.UserID,
) (map[ids.UserID]port.PersonReturn, error) {
	out := make(map[ids.UserID]port.PersonReturn, len(users))
	if len(users) == 0 {
		return out, nil
	}
	subjects := make([]uuid.UUID, len(users))
	for i, user := range users {
		subjects[i] = user.UUID()
	}
	rows, err := sqlc.New(l.DB).BoardRowsForSubjects(ctx, sqlc.BoardRowsForSubjectsParams{
		Board: string(domain.BoardPeople), Range: string(rng), SubjectIds: subjects,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "ranking.Latest.PeopleReturns")
	}
	for _, row := range rows {
		person := port.PersonReturn{PnlMicros: money.SignedMicrosFromInt64(row.PnlMicros), AsOf: row.PricesAsOf}
		if row.ReturnBps.Valid {
			bps := row.ReturnBps.Int64
			person.ReturnBps = &bps
		}
		out[ids.UserIDFrom(row.SubjectID)] = person
	}
	return out, nil
}
