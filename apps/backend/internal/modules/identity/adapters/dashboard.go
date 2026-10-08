package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
)

type Dashboard struct {
	q *sqlc.Queries
}

var _ port.Dashboard = Dashboard{}

func NewDashboard(db sqlc.DBTX) Dashboard { return Dashboard{q: sqlc.New(db)} }

func (d Dashboard) StatusCounts(ctx context.Context) ([]port.StatusCount, error) {
	rows, err := d.q.UserStatusCounts(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "identity.Dashboard.StatusCounts")
	}
	out := make([]port.StatusCount, len(rows))
	for i, r := range rows {
		out[i] = port.StatusCount{AuthState: r.AuthState, AccountStatus: r.AccountStatus, Users: r.Users}
	}
	return out, nil
}
