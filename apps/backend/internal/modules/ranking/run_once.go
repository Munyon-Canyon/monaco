package ranking

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
)

func (m *Module) RunOnce(ctx context.Context) error {
	ctx = auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: "ranking.seed"})
	now := m.deps.Clock.Now()
	valuation, err := m.RunValuation().Run(ctx, now)
	if err != nil {
		return err
	}
	_, err = app.NewSnapshotWriter(m.deps.UoW, m.deps.IDs).Write(ctx, valuation, now, m.deps.Clock.Now())
	return err
}
