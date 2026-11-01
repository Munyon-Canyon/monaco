package treasury

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
)

func (m *Module) FundRefunds() *app.FundRefunds {
	cfg := m.deps.Config
	return app.NewFundRefunds(app.FundRefundDeps{
		Reads: m.deps.Pool, UoW: m.deps.UoW, IDs: m.deps.IDs, Clock: m.deps.Clock, Ledger: m.ledger(),
		Pot: m.strictReads(), Chain: adapters.NewStatuses(cfg, m.deps.Clock),
		Transfers: m.lazyTransfers(),
		Wallets:   app.PayoutWalletReads{Cabals: m.treasuryWallets, Members: m.memberWallets},
		Balances:  solana.New(cfg, m.deps.Clock),
		USDC:      chain.Mint{Address: chain.SolanaAddress(cfg.Solana.USDCMint), Decimals: usdcDecimals},
		Wait:      app.FundRefundWait, Poll: app.FundRefundPoll,
	})
}
