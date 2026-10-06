package adapters

import (
	"context"
	"database/sql"
	"errors"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/agents/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/agents/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/agents/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Postgres struct {
	q *sqlc.Queries
}

var _ port.Queries = Postgres{}

func NewQueries(db sqlc.DBTX) Postgres { return Postgres{q: sqlc.New(db)} }

func (r Postgres) AgentOf(ctx context.Context, cabalID ids.CabalID) (port.Agent, bool, error) {
	const op = "agents.AgentOf"
	row, err := r.q.LiveAgentOfCabal(ctx, cabalID.UUID())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return port.Agent{}, false, nil
	case err != nil:
		return port.Agent{}, false, errs.Wrap(err, errs.CodeInternal, op)
	case row.BudgetUsdcMicros < 0:
		return port.Agent{}, false, errs.New(errs.CodeDecodeFailed, op)
	}
	return port.Agent{
		ID:           ids.AgentIDFrom(row.ID),
		CabalID:      ids.CabalIDFrom(row.CabalID),
		Name:         row.Name,
		Status:       domain.Status(row.Status),
		BudgetMicros: money.MicrosFromUint64(uint64(row.BudgetUsdcMicros)),
	}, true, nil
}
