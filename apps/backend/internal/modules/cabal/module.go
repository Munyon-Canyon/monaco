package cabal

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type multichainParts struct {
	evmWallets multichain.EVMWallets
	forwarding multichain.DepositForwarding
}

func newMultichain(d module.Deps) multichainParts {
	parts := multichainParts{
		evmWallets: multichain.Pending{Ticket: "#4966"},
		forwarding: adapters.MultichainDepositsDepositForwarding(),
	}
	if client, err := adapters.MultichainPrivyEVMWallets(d.Config, d.Clock); err == nil {
		parts.evmWallets = client
	}
	return parts
}

type Module struct {
	multichain multichainParts
	deps       module.Deps
	wallets    app.TreasuryWallets
	treasury   app.TreasuryReads
	unread     app.UnreadCounter
	votes      app.OpenVotes
	codes      app.CodeResolver
	calls      app.MemberCalls
	blocks     app.Blocks
}

type liveReads struct {
	reads interface {
		ShareUnits(ctx context.Context, cabalID ids.CabalID, user ids.UserID) (money.SharesUnits, error)
		OpenFund(ctx context.Context, cabalID ids.CabalID, user ids.UserID) (bool, error)
		PotValue(ctx context.Context, cabalID ids.CabalID) (money.Micros, error)
		Ownership(ctx context.Context, cabalID ids.CabalID) (map[ids.UserID]int32, error)
	}
	sharesOn func(ctx context.Context, db dbsqlc.DBTX, cabalID ids.CabalID, user ids.UserID) (money.SharesUnits, error)
	openOn   func(ctx context.Context, db dbsqlc.DBTX, cabalID ids.CabalID, user ids.UserID) (bool, error)
}

func (l liveReads) ShareUnits(ctx context.Context, cabalID ids.CabalID, user ids.UserID) (money.SharesUnits, error) {
	return l.reads.ShareUnits(ctx, cabalID, user)
}

func (l liveReads) OpenFund(ctx context.Context, cabalID ids.CabalID, user ids.UserID) (bool, error) {
	return l.reads.OpenFund(ctx, cabalID, user)
}

func (l liveReads) PotValue(ctx context.Context, cabalID ids.CabalID) (money.Micros, error) {
	return l.reads.PotValue(ctx, cabalID)
}

func (l liveReads) Ownership(ctx context.Context, cabalID ids.CabalID) (map[ids.UserID]int32, error) {
	return l.reads.Ownership(ctx, cabalID)
}

func (l liveReads) ShareUnitsOn(
	ctx context.Context, db dbsqlc.DBTX, cabalID ids.CabalID, user ids.UserID,
) (money.SharesUnits, error) {
	return l.sharesOn(ctx, db, cabalID, user)
}

func (l liveReads) OpenFundOn(ctx context.Context, db dbsqlc.DBTX, cabalID ids.CabalID, user ids.UserID) (bool, error) {
	return l.openOn(ctx, db, cabalID, user)
}

type Option func(*Module)

func WithTreasuryWallets(wallets app.TreasuryWallets) Option {
	return func(m *Module) { m.wallets = wallets }
}

func WithTreasuryReads(reads app.TreasuryReads) Option {
	return func(m *Module) { m.treasury = reads }
}

func WithEVMWallets(wallets multichain.EVMWallets) Option {
	return func(m *Module) { m.multichain.evmWallets = wallets }
}

func New(d module.Deps, opts ...Option) *Module {
	reads := treasury.New(d).LiveReads()
	m := &Module{multichain: newMultichain(d), deps: d, treasury: liveReads{
		reads: reads,
		sharesOn: func(ctx context.Context, db dbsqlc.DBTX, cabalID ids.CabalID, user ids.UserID) (money.SharesUnits, error) {
			return reads.On(db).ShareUnits(ctx, cabalID, user)
		},
		openOn: func(ctx context.Context, db dbsqlc.DBTX, cabalID ids.CabalID, user ids.UserID) (bool, error) {
			return reads.On(db).OpenFund(ctx, cabalID, user)
		},
	}}
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
		if provider, ok := mod.(interface{ Codes() app.CodeResolver }); ok {
			m.codes = provider.Codes()
		}
		if provider, ok := mod.(interface{ Calls() rankingport.Calls }); ok {
			m.calls = provider.Calls()
		}
		if provider, ok := mod.(interface{ Blocks() app.Blocks }); ok {
			m.blocks = provider.Blocks()
		}
		if provider, ok := mod.(interface{ Queries() governanceport.Queries }); ok {
			if votes, ok := provider.Queries().(app.OpenVotes); ok {
				m.votes = votes
			}
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
	votes := m.votes
	if votes == nil {
		votes = app.NoOpenVotes{}
	}
	calls := m.calls
	if calls == nil {
		calls = app.NoCalls{}
	}
	var blocks app.Blocks = app.NoBlocks{}
	if m.blocks != nil {
		blocks = m.blocks
	}
	var ownership app.MemberOwnership = app.NoOwnership{}
	if reads, ok := m.treasury.(app.MemberOwnership); ok {
		ownership = reads
	}
	return adapters.HTTP{
		Ownership: ownership, Calls: calls,
		Create:  m.CreateCabalHandler(),
		Join:    app.NewJoinCabalHandler(m.deps.UoW, m.deps.Clock),
		Request: app.NewRequestAccessHandler(m.deps.UoW, m.deps.IDs, m.deps.Clock, m.codes),
		Revoke:  app.NewRevokeAccessHandler(m.deps.UoW, m.deps.Clock),
		Decide:  app.NewDecideAccessHandler(m.deps.UoW, blocks, m.deps.Clock),
		Update:  app.NewUpdateCabalHandler(m.deps.UoW, m.deps.Clock),
		Picture: app.NewSetCabalPictureHandler(m.deps.UoW, m.deps.Pool, m.deps.IDs, m.deps.Clock, m.deps.Photos),
		Unpic:   app.NewRemoveCabalPictureHandler(m.deps.UoW, m.deps.IDs, m.deps.Clock),
		Leave:   m.LeaveCabalHandler(),
		Reorder: app.NewReorderCabalsHandler(m.deps.UoW),
		Invite:  app.NewInviteMemberHandler(m.deps.UoW, users, blocks, m.deps.IDs, m.deps.Clock),
		DB:      m.deps.Pool, Users: users, Unread: unread, Votes: votes, Clock: m.deps.Clock,
	}
}

func (m *Module) Consumers() []bus.Consumer {
	hints := adapters.Hints{Publish: m.deps.Bus}
	evm := adapters.TreasuryEVMWalletOnCreate{Wallets: m.TreasuryEVMWallets(), UoW: m.deps.UoW}
	return []bus.Consumer{{
		Durable:  "cabal_rule_passed",
		Handlers: []bus.HandlerSpec{bus.Handle("cabal.rule_passed", adapters.RulePassed{}.Handle)},
	}, {
		Durable:  "cabal_ban",
		Handlers: []bus.HandlerSpec{bus.Handle("cabal.ban", adapters.Ban{IDs: m.deps.IDs}.Handle)},
	}, {
		Durable: "cabal_hints",
		Handlers: []bus.HandlerSpec{
			bus.Handle("cabal.hints", hints.Handle),
			bus.Handle("cabal.hints.member_joined", hints.MemberJoined),
			bus.Handle("cabal.hints.access_requested", hints.AccessRequested),
			bus.Handle("cabal.hints.access_decided", hints.AccessDecided),
			bus.Handle("cabal.hints.updated", hints.Updated),
			bus.Handle("cabal.hints.member_left", hints.MemberLeft),
		},
	}, {
		Durable:  "cabal_evm_wallet",
		Handlers: []bus.HandlerSpec{bus.HandleOwn("cabal.evm_wallet", evm.Handle)},
	}}
}

func (m *Module) Pollers() []poller.Poller {
	return []poller.Poller{app.NewInviteExpiryPoller(m.deps.UoW, m.deps.Pool, m.deps.Clock)}
}

func (m *Module) Queries() port.Queries { return adapters.NewQueries(m.deps.Pool) }

func (m *Module) Invites() adapters.Postgres {
	return adapters.NewQueries(m.deps.Pool).WithUsers(identity.New(m.deps).Queries())
}

func (m *Module) TreasuryEVM() port.TreasuryEVM {
	return port.NewTreasuryEVM(m.TreasuryEVMWallets().EnsureTreasuryEVMWallet)
}

func (m *Module) TreasuryEVMWallets() adapters.TreasuryEVMWallets {
	return adapters.TreasuryEVMWallets{DB: m.deps.Pool, Wallets: m.multichain.evmWallets, Clock: m.deps.Clock}
}

func (m *Module) EVMWalletBackfillCabals(ctx context.Context) ([]app.EVMWalletBackfillCabal, error) {
	return app.EVMWalletBackfillCabals(ctx, m.deps.Pool)
}

func (*Module) DashboardOn(db dbsqlc.DBTX) adapters.Dashboard { return adapters.NewDashboard(db) }

func (m *Module) LeaveCabalHandler() *app.LeaveCabalHandler {
	return app.NewLeaveCabalHandler(m.deps.UoW, m.deps.Pool, m.treasury)
}

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
	View              = port.CabalView
	LeaveCabal        = app.LeaveCabal
	LeaveCabalHandler = app.LeaveCabalHandler
	MemberView        = port.MemberView
	Rules             = port.Rules
	TreasuryWallet    = port.TreasuryWallet
	Status            = port.Status
	JoinMode          = domain.JoinMode
	CodeResolver      = app.CodeResolver
	VoterMode         = domain.VoterMode
	Threshold         = domain.Threshold
	Role              = domain.Role
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
