package funding

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct{}

func New(module.Deps) *Module { return &Module{} }

func (*Module) Name() string { return "funding" }

func (*Module) Routes(*httpx.Routes) {}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{}
}

func (*Module) Pollers() []poller.Poller { return nil }

func (*Module) Balances() port.Balances { return adapters.UnwiredBalances{} }

type (
	Balances = port.Balances
	Balance  = port.Balance
)
