package treasury

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/tools/ops/replay"
)

const usdcMainnet = domain.Asset("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")

type Module struct {
	deps module.Deps
}

type Queries = port.Queries

type Position = port.Position

type Stake = port.Stake

type CabalPositions = port.CabalPositions

type MemberStake = port.MemberStake

func New(d module.Deps) *Module { return &Module{deps: d} }

func (*Module) Name() string { return "treasury" }

func (*Module) Routes(*httpx.Routes) {}

func (m *Module) Consumers() []bus.Consumer {
	return []bus.Consumer{
		{
			Durable: "treasury_trades",
			Handlers: []bus.HandlerSpec{
				bus.Handle("treasury.trades", adapters.Trades{Ledger: m.ledger()}.Handle),
			},
		},
	}
}

func (*Module) Pollers() []poller.Poller { return nil }

func (*Module) Queries() port.Queries { return adapters.Unwired{} }

func (m *Module) ledger() app.Ledger {
	return app.NewLedger(chain.SolanaAddress(usdcMainnet), m.deps.Clock)
}

func LedgerCheck() replay.LedgerCheck {
	return replay.LedgerCheck{
		Name: "treasury",
		Tables: []string{
			"cabal_txns", "cabal_txn_entries", "user_txns", "user_txn_entries", "cabal_positions", "user_positions",
		},
		Check: adapters.CheckLedger(usdcMainnet, nil),
	}
}
