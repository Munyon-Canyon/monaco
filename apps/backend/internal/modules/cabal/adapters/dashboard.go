package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
)

type Dashboard struct {
	q *sqlc.Queries
}

var _ port.Dashboard = Dashboard{}

func NewDashboard(db sqlc.DBTX) Dashboard { return Dashboard{q: sqlc.New(db)} }

func (d Dashboard) Counts(ctx context.Context) (port.Counts, error) {
	row, err := d.q.CabalCounts(ctx)
	if err != nil {
		return port.Counts{}, errs.Wrap(err, errs.CodeOf(err), "cabal.Dashboard.Counts")
	}
	return port.Counts{
		Cabals: row.Cabals, Banned: row.Banned, MembersP50: row.MembersP50, MembersP90: row.MembersP90,
		MembersMax: row.MembersMax,
	}, nil
}
