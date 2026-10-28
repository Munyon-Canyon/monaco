package adapters

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

var _ port.CabalReturnsReader = Latest{}

func (l Latest) CabalReturns(
	ctx context.Context, rng domain.Range, cabals []ids.CabalID,
) (map[ids.CabalID]port.CabalReturn, error) {
	out := make(map[ids.CabalID]port.CabalReturn, len(cabals))
	if len(cabals) == 0 {
		return out, nil
	}
	subjects := make([]uuid.UUID, len(cabals))
	for i, cabal := range cabals {
		subjects[i] = cabal.UUID()
	}
	rows, err := sqlc.New(l.DB).BoardRowsForSubjects(ctx, sqlc.BoardRowsForSubjectsParams{
		Board: string(domain.BoardCabals), Range: string(rng), SubjectIds: subjects,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "ranking.Latest.CabalReturns")
	}
	for _, row := range rows {
		found := port.CabalReturn{AsOf: row.PricesAsOf}
		if row.ReturnBps.Valid {
			bps := row.ReturnBps.Int64
			found.ReturnBps = &bps
		}
		out[ids.CabalIDFrom(row.SubjectID)] = found
	}
	return out, nil
}
