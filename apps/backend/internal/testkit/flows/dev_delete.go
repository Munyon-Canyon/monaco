package flows

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	chainprivy "github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func DeleteDevUser(ctx context.Context, cfg config.Config, id ids.UserID, balances app.WalletBalances) error {
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer pool.Close()
	clk := clock.Real{}
	client, err := chainprivy.New(cfg, clk)
	if err != nil {
		return err
	}
	return app.DeleteDevUser(ctx, app.DeleteDevUserDeps{
		UoW: db.New(pool, ids.Real{}, clk), Users: adapters.Users{}, Privy: privyadapter.Users{Client: client},
		Balances: balances,
	}, id)
}
