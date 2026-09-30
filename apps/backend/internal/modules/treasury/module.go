package treasury

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/tools/ops/replay"
)

const usdcMainnet = domain.Asset("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")

type Module struct{}

func New(module.Deps) *Module { return &Module{} }

func (*Module) Name() string { return "treasury" }

func (*Module) Routes(*httpx.Routes) {}

func (*Module) Consumers() []bus.Consumer { return nil }

func (*Module) Pollers() []poller.Poller { return nil }

func LedgerCheck() replay.LedgerCheck {
	return replay.LedgerCheck{
		Name: "treasury",
		Tables: []string{
			"cabal_txns", "cabal_txn_entries", "user_txns", "user_txn_entries", "cabal_positions", "user_positions",
		},
		Check: adapters.CheckLedger(usdcMainnet, nil),
	}
}
