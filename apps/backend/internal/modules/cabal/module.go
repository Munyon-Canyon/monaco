package cabal

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps     module.Deps
	wallets  app.TreasuryWallets
	treasury app.TreasuryReads
	unread   app.UnreadCounter
}

type Option func(*Module)

func WithTreasuryWallets(wallets app.TreasuryWallets) Option {
	return func(m *Module) { m.wallets = wallets }
}

func WithTreasuryReads(reads app.TreasuryReads) Option {
	return func(m *Module) { m.treasury = reads }
}

func New(d module.Deps, opts ...Option) *Module {
	m := &Module{deps: d, treasury: treasury.New(d).Queries()}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Module) Wire(set module.Set) {
	for _, mod := range set {
		if provider, ok := mod.(interface{ Chat() app.UnreadCounter }); ok {
			m.unread = provider.Chat()
		}
	}
}

func (*Module) Name() string { return "cabal" }

func (m *Module) Mount(r api.Mount) { cabalapi.Mount(m.http(), r) }

func (m *Module) http() adapters.HTTP {
	users := identity.New(m.deps).Queries()
	unread := m.unread
	if unread == nil {
		unread = app.UnwiredUnread{}
	}
	return adapters.HTTP{
		Create:  m.CreateCabalHandler(),
		Join:    app.NewJoinCabalHandler(m.deps.UoW),
		Request: app.NewRequestAccessHandler(m.deps.UoW, m.deps.IDs, m.deps.Clock),
		Revoke:  app.NewRevokeAccessHandler(m.deps.UoW, m.deps.Clock),
		Decide:  app.NewDecideAccessHandler(m.deps.UoW, m.deps.Clock),
		Update:  app.NewUpdateCabalHandler(m.deps.UoW, m.deps.Clock),
		Picture: app.NewSetCabalPictureHandler(m.deps.UoW, m.deps.Pool, m.deps.IDs, m.deps.Clock, m.deps.Photos),
		Leave:   app.NewLeaveCabalHandler(m.deps.UoW, m.deps.Pool, m.treasury),
		Invite:  app.NewInviteMemberHandler(m.deps.UoW, users, m.deps.IDs, m.deps.Clock),
		DB:      m.deps.Pool, Users: users, Unread: unread, Clock: m.deps.Clock,
	}
}

func (m *Module) Consumers() []bus.Consumer {
	hints := adapters.Hints{Publish: m.deps.Bus}
	return []bus.Consumer{{
		Durable: "cabal_hints",
		Handlers: []bus.HandlerSpec{
			bus.Handle("cabal.hints", hints.Handle),
			bus.Handle("cabal.hints.member_joined", hints.MemberJoined),
			bus.Handle("cabal.hints.access_requested", hints.AccessRequested),
			bus.Handle("cabal.hints.access_decided", hints.AccessDecided),
			bus.Handle("cabal.hints.updated", hints.Updated),
			bus.Handle("cabal.hints.member_left", hints.MemberLeft),
		},
	}}
}

func (m *Module) Pollers() []poller.Poller {
	return []poller.Poller{app.NewInviteExpiryPoller(m.deps.UoW, m.deps.Pool, m.deps.Clock)}
}

func (m *Module) Queries() port.Queries { return adapters.NewQueries(m.deps.Pool) }

func (m *Module) CreateCabalHandler() *app.CreateCabalHandler {
	wallets := m.wallets
	if wallets == nil {
		client, err := privy.New(m.deps.Config, m.deps.Clock)
		if err != nil {
			panic(err)
		}
		wallets = adapters.AppWallets{Client: client}
	}
	return app.NewCreateCabalHandler(app.CreateCabalDeps{
		UoW: m.deps.UoW, Reads: m.deps.Pool, Wallets: wallets, IDs: m.deps.IDs, Clock: m.deps.Clock,
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

	JoinRequest = domain.JoinRequest

	VotersAll  = domain.VotersAll
	VotersList = domain.VotersList

	ThresholdMajority  = domain.ThresholdMajority
	ThresholdUnanimous = domain.ThresholdUnanimous

	RoleCreator = domain.RoleCreator
	RoleMember  = domain.RoleMember
)

type Queries = port.Queries
