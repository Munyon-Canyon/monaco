package cabal

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps    module.Deps
	wallets app.TreasuryWallets
}

type Option func(*Module)

func WithTreasuryWallets(wallets app.TreasuryWallets) Option {
	return func(m *Module) { m.wallets = wallets }
}

func New(d module.Deps, opts ...Option) *Module {
	m := &Module{deps: d}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (*Module) Name() string { return "cabal" }

func (*Module) Routes(*httpx.Routes) {}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{}
}

func (*Module) Pollers() []poller.Poller { return nil }

func (m *Module) Queries() port.Queries { return adapters.NewQueries(m.deps.Pool) }

func (m *Module) CreateCabalHandler() *app.CreateCabalHandler {
	wallets := m.wallets
	if wallets == nil {
		wallets = adapters.AppWallets{Client: privy.New(m.deps.Config, m.deps.Clock)}
	}
	return app.NewCreateCabalHandler(app.CreateCabalDeps{
		UoW: m.deps.UoW, Wallets: wallets, IDs: m.deps.IDs, Clock: m.deps.Clock,
	})
}

type (
	View           = port.CabalView
	MemberView     = port.MemberView
	Rules          = port.Rules
	TreasuryWallet = port.TreasuryWallet
	Status         = port.Status
	JoinMode       = domain.JoinMode
	VoterMode      = domain.VoterMode
	Threshold      = domain.Threshold
	Role           = domain.Role
)

const (
	StatusActive = port.StatusActive
	StatusBanned = port.StatusBanned

	JoinOpen    = domain.JoinOpen
	JoinRequest = domain.JoinRequest

	VotersAll  = domain.VotersAll
	VotersList = domain.VotersList

	ThresholdMajority  = domain.ThresholdMajority
	ThresholdUnanimous = domain.ThresholdUnanimous

	RoleCreator = domain.RoleCreator
	RoleMember  = domain.RoleMember
)

type Queries = port.Queries
