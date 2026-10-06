package adapters

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
)

func CheckPauses(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := sqlc.New(pool).PausesOnSettledExternalDeposits(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "funding.CheckPauses")
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, "pause "+r.PauseID.String()+" is open on "+r.Status+" external deposit "+
			r.ExternalDepositID.String())
	}
	return out, nil
}
