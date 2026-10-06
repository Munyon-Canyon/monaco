package port

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/agents/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Agent struct {
	ID           ids.AgentID
	CabalID      ids.CabalID
	Name         string
	Status       domain.Status
	BudgetMicros money.Micros
}

type Queries interface {
	AgentOf(ctx context.Context, cabalID ids.CabalID) (Agent, bool, error)
}
