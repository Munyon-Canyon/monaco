package treasury

import (
	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/tools/ops/replay"
)

type Module struct {
	deps    module.Deps
	members app.Members
	users   app.Users
}

type Queries = port.Queries

type Position = port.Position

type Stake = port.Stake

type CabalPositions = port.CabalPositions

type MemberStake = port.MemberStake

func New(d module.Deps) *Module {
	return &Module{deps: d, members: app.UnwiredReads{}, users: app.UnwiredReads{}}
}

func (*Module) Name() string { return "treasury" }

func (m *Module) Wire(set module.Set) {
	for _, mod := range set {
		switch provider := mod.(type) {
		case interface{ Queries() cabalport.Queries }:
			m.members = provider.Queries()
		case interface{ Queries() identityport.Queries }:
			m.users = provider.Queries()
		}
	}
}

func (m *Module) Routes(r *httpx.Routes) {
	names := catalogNames{Catalog: market.New(m.deps).Catalog()}
	r.TreasuryRoutes = adapters.HTTP{Reads: app.NewActivityReads(m.deps.Pool, m.members, m.users, names)}
}

func (m *Module) Consumers() []bus.Consumer {
	activity := adapters.Activity{Hints: m.deps.Bus}
	userLedger := adapters.UserLedger{Ledger: m.ledger(), IDs: m.deps.IDs, USDC: usdc(m.deps.Config)}
	return []bus.Consumer{
		{
			Durable: "treasury_trades",
			Handlers: []bus.HandlerSpec{
				bus.Handle("treasury.trades", adapters.Trades{Ledger: m.ledger()}.Handle),
			},
		},
		{
			Durable: "treasury_activity",
			Handlers: []bus.HandlerSpec{
				bus.Handle("treasury.activity.submitted", activity.Submitted),
				bus.Handle("treasury.activity.confirmed", activity.Confirmed),
				bus.Handle("treasury.activity.failed", activity.Failed),
			},
		},
		{
			Durable: "treasury_user_ledger",
			Handlers: []bus.HandlerSpec{
				bus.Handle("treasury.user_ledger", userLedger.Handle),
			},
		},
	}
}

func (*Module) Pollers() []poller.Poller { return nil }

func (*Module) Queries() port.Queries { return adapters.Unwired{} }

func (m *Module) ledger() app.Ledger {
	return app.NewLedger(chain.SolanaAddress(m.deps.Config.Solana.USDCMint), m.deps.Clock)
}

func usdc(cfg config.Config) domain.Asset {
	return domain.MintAsset(chain.SolanaAddress(cfg.Solana.USDCMint))
}

func LedgerCheck(cfg config.Config) replay.LedgerCheck {
	return replay.LedgerCheck{
		Name: "treasury",
		Tables: []string{
			"cabal_txns", "cabal_txn_entries", "user_txns", "user_txn_entries", "cabal_positions", "user_positions",
		},
		Check: adapters.CheckLedger(usdc(cfg), map[events.Type]adapters.BalanceRule{
			events.TypeDepositCredited: adapters.DepositCreditedBalances(usdc(cfg)),
		}),
	}
}
