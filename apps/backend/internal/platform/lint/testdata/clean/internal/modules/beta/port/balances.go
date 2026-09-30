package port

import "github.com/monaco/monaco/apps/backend/internal/modules/beta/domain"

type Balances interface {
	Balance() domain.Balance
}
