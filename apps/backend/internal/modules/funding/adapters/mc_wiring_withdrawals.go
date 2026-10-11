package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func MultichainWithdrawalsWalletActions(cfg config.Config, clk clock.Clock) PrivyWalletActions {
	return PrivyWalletActions{cfg: cfg, clk: clk}
}

type PrivyWalletActions struct {
	cfg config.Config
	clk clock.Clock
}

func (p PrivyWalletActions) Transfer(
	ctx context.Context, walletID string, spec multichain.TransferSpec, idemKey string,
) (multichain.ActionID, error) {
	client, err := privy.New(p.cfg, p.clk)
	if err != nil {
		return "", err
	}
	return client.Transfer(ctx, walletID, spec, idemKey)
}

func (p PrivyWalletActions) Action(
	ctx context.Context, walletID string, id multichain.ActionID,
) (multichain.ActionStatus, error) {
	client, err := privy.New(p.cfg, p.clk)
	if err != nil {
		return multichain.ActionStatus{}, err
	}
	return client.Action(ctx, walletID, id)
}
