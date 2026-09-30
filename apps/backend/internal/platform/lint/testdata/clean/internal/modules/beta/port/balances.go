package port

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/beta/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/beta/sqlc"
)

type Balances interface {
	Balance() domain.Balance
}

func BalanceOf(q sqlc.Queries) domain.Balance {
	return domain.Balance{Micros: q.Micros}
}
