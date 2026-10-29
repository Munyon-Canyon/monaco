package governance

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/monaco/monaco/apps/backend/internal/modules/admin"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const meterName = "github.com/monaco/monaco/apps/backend/internal/modules/governance"

type (
	Status     = domain.Status
	ProposalID = ids.ProposalID
)

type Port = port.Queries

var _ Port = app.Queries{}

type Ports = app.TradePorts

type Option func(*Module)

func WithPorts(p Ports) Option { return func(m *Module) { m.ports = &p } }

func WithSwaps(s app.Swaps) Option { return func(m *Module) { m.swaps = s } }

type Module struct {
	deps  module.Deps
	ports *Ports
	swaps app.Swaps
}

func New(d module.Deps, opts ...Option) *Module {
	m := &Module{deps: d}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (*Module) Name() string { return "governance" }

func (m *Module) Mount(r api.Mount) {
	ports := m.tradePorts()
	hints := adapters.Hints{Publish: m.deps.Bus}
	reads := m.proposalReads(ports)
	pending := m.pendingReads(reads)
	facts := app.NewMarketFacts(
		m.deps.Pool, cabal.New(m.deps).Queries(), treasury.New(m.deps).Held(), identity.New(m.deps).Queries(),
		m.deps.Clock,
	)
	propose := app.NewProposeTradeHandler(m.deps.UoW, m.deps.IDs, m.deps.Clock, ports, hints)
	governanceapi.Mount(adapters.HTTP{
		Propose: propose,
		RuleChange: app.NewProposeRuleChangeHandler(
			m.deps.UoW, m.deps.IDs, m.deps.Clock, cabal.New(m.deps).Queries(), hints,
		),
		Facts: facts,
		Comparisons: app.NewProposalComparisons(
			m.deps.Pool, propose, adapters.PinListings{Market: market.New(m.deps).Catalog()},
		),
		Vote: app.NewCastVoteHandler(m.deps.UoW, ports.Cabals, m.deps.Clock, hints).Priced(
			market.New(m.deps).Catalog(), market.New(m.deps).Prices(),
		),
		RetractVote: app.NewRetractVoteHandler(m.deps.UoW, m.deps.Clock, hints),
		Reminders:   app.NewReminderHandler(m.deps.UoW, m.deps.Clock),
		Withdraw:    app.NewWithdrawProposalHandler(m.deps.UoW, m.deps.Clock, hints),
		Void: app.NewVoidProposalHandler(
			m.deps.UoW, m.deps.Pool, m.deps.IDs, m.deps.Clock, trading.New(m.deps).Queries(), hints,
		),
		Reads:   reads,
		Pending: pending,
		Admin: app.NewAdminReads(
			reads, bus.NewEventLog(m.deps.Pool, m.deps.Clock), identity.New(m.deps).Queries(),
			cabal.New(m.deps).Queries(), admin.New(m.deps).Actions(),
		),
	}, r)
}

func (m *Module) proposalReads(ports Ports) *app.ProposalReads {
	return app.NewProposalReads(
		m.deps.Pool, trading.New(m.deps).Queries(), cabal.New(m.deps).Queries(), ports.Assets,
		identity.New(m.deps).Queries(), ranking.New(m.deps).Queries(), social.New(m.deps),
	).Priced(market.New(m.deps).Prices(), m.deps.Clock)
}

func (m *Module) pendingReads(reads *app.ProposalReads) *app.PendingVoteReads {
	return app.NewPendingVoteReads(reads, app.PendingVotePorts{
		Assets: market.New(m.deps).Catalog(), Replies: social.New(m.deps),
	})
}

func (m *Module) tradePorts() Ports {
	if m.ports != nil {
		return *m.ports
	}
	deps := m.deps
	if deps.Config.Jupiter.SwapBaseURL == "" {
		deps.Config.Jupiter.SwapBaseURL = "http://jupiter.invalid"
	}
	if deps.Config.Jupiter.PriceBaseURL == "" {
		deps.Config.Jupiter.PriceBaseURL = "http://jupiter.invalid"
	}
	if deps.Config.Solana.RPCURL == "" {
		deps.Config.Solana.RPCURL = "http://solana.invalid"
	}
	if deps.Config.Timeouts.RPC == 0 {
		deps.Config.Timeouts.RPC = time.Second
	}
	if deps.Config.Timeouts.JupiterQuote == 0 {
		deps.Config.Timeouts.JupiterQuote = time.Second
	}
	if deps.Config.Timeouts.JupiterExecute == 0 {
		deps.Config.Timeouts.JupiterExecute = time.Second
	}
	if deps.Config.Privy.BaseURL == "" {
		deps.Config.Privy.BaseURL = "http://privy.invalid"
	}
	if deps.Config.Timeouts.Privy == 0 {
		deps.Config.Timeouts.Privy = time.Second
	}
	markets := market.New(deps)
	return Ports{
		Cabals: cabal.New(m.deps).Queries(), Assets: markets.Catalog(), Routes: markets.RouteChecker(),
		Router: markets.Router(), Listings: markets.Catalog(), Quoter: markets.Quoter(),
		Treasury: treasury.New(m.deps).Queries(), Balances: solana.New(deps.Config, deps.Clock),
		USDC: chain.Mint{Address: chain.SolanaAddress(deps.Config.Solana.USDCMint), Decimals: 6},
	}
}

func (m *Module) Consumers() []bus.Consumer {
	hints := adapters.Hints{Publish: m.deps.Bus}
	swaps := m.swaps
	if swaps == nil {
		swaps = trading.New(m.deps).Queries()
	}
	outcomes := adapters.TradeOutcome{Hints: hints, Swaps: swaps}
	return []bus.Consumer{
		{
			Durable: "governance",
			Handlers: []bus.HandlerSpec{
				bus.Handle("governance.trade_outcome.confirmed", outcomes.Confirmed),
				bus.Handle("governance.trade_outcome.blocked", outcomes.Blocked),
				bus.Handle("governance.trade_outcome.failed", outcomes.Failed),
				bus.Handle("governance.trade_outcome.retried", outcomes.Retried),
				bus.Handle("governance.ban_voids", adapters.BanVoids{Voids: m.voidProposal(hints)}.Handle),
			},
		},
	}
}

func (m *Module) Pollers() []poller.Poller {
	hints := adapters.Hints{Publish: m.deps.Bus}
	app.ObserveOpenProposals(otel.GetMeterProvider().Meter(meterName), app.NewDashboard(m.deps.Pool))
	return []poller.Poller{
		app.NewExpiryPoller(m.deps.UoW, m.deps.Pool, m.deps.Clock, hints),
		app.NewCloseReminderPoller(m.deps.UoW, m.deps.Pool, m.deps.Clock),
	}
}

func (m *Module) CatchUp() adapters.CatchUp {
	return adapters.NewCatchUp(m.pendingReads(m.proposalReads(m.tradePorts())))
}

func (m *Module) Queries() port.Queries { return app.NewQueries(m.deps.Pool) }

func (*Module) DashboardOn(db dbsqlc.DBTX) app.Dashboard { return app.NewDashboard(db) }

func (m *Module) Voters() port.Voters { return app.NewQueries(m.deps.Pool) }

func (m *Module) Tallies() port.Tallies { return app.NewQueries(m.deps.Pool) }

func (m *Module) MemberActivity() port.MemberActivity { return app.NewQueries(m.deps.Pool) }

func (m *Module) Ballots() port.Ballots { return adapters.NewBallots(m.deps.Pool) }

func (m *Module) VoteStates() adapters.VoteStates { return adapters.NewVoteStates(m.deps.Pool) }

func (m *Module) ProposalHeads() adapters.ProposalHeads {
	return adapters.NewProposalHeads(m.deps.Pool)
}

func (m *Module) InviteFacts() app.Queries { return app.NewQueries(m.deps.Pool) }

func (m *Module) ProposedMints(ctx context.Context) ([]chain.SolanaAddress, error) {
	return adapters.NewProposedMints(m.deps.Pool).ProposedMints(ctx)
}

func (m *Module) VoidFromOps(ctx context.Context, id ProposalID, rawReason string) error {
	reason, err := domain.ParseVoidReason(rawReason)
	if err != nil {
		return err
	}
	ctx = auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: "monacoctl"})
	voids := app.NewVoidProposalHandler(
		m.deps.UoW,
		m.deps.Pool,
		m.deps.IDs,
		m.deps.Clock,
		trading.New(m.deps).Queries(),
		adapters.Hints{Publish: m.deps.Bus},
	)
	return voids.Handle(ctx, app.VoidProposal{ProposalID: id, Reason: reason})
}

func (m *Module) voidProposal(hints app.Hints) *app.VoidProposalHandler {
	return app.NewVoidProposalHandler(
		m.deps.UoW, m.deps.Pool, m.deps.IDs, m.deps.Clock, trading.New(m.deps).Queries(), hints,
	)
}
