package adapters

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/beta/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/beta/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/beta/sqlc"
)

type Postgres struct {
	q sqlc.Queries
}

var _ port.Balances = Postgres{}

func (p Postgres) Balance() domain.Balance {
	return domain.Balance{Micros: p.q.Micros}
}
