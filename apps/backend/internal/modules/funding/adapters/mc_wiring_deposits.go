package adapters

import (
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func MultichainDepositsDepositForwarding(cfg config.Config, clk clock.Clock) (*privy.Client, error) {
	return privy.New(cfg, clk)
}
