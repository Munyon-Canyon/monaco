package treasury

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type CatalogNames = catalogNames

func (m *Module) Reads() (members, users any) { return m.members, m.users }

func (m *Module) CashOutPause(ctx context.Context, cabal ids.CabalID) (app.CashOutPause, error) {
	return m.pauses.IsPaused(ctx, cabal)
}
