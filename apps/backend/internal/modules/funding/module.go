package funding

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps     module.Deps
	balances port.Balances
	owners   []app.SignatureOwner
}

func New(d module.Deps) *Module { return &Module{deps: d} }

func (*Module) Name() string { return "funding" }

func (m *Module) Wire(set module.Set) {
	for _, mod := range set {
		if provider, ok := mod.(interface {
			SignatureOwner() chain.SignatureOwnerFunc
		}); ok {
			m.owners = append(m.owners, provider.SignatureOwner())
		}
	}
}

func (m *Module) Routes(r *httpx.Routes) {
	r.FundingRoutes = adapters.HTTP{
		Balances: m.Balances(), Wallets: app.WalletReader{Reader: identity.New(m.deps).Queries()},
	}
}

func (m *Module) Consumers() []bus.Consumer {
	cfg := m.deps.Config
	resolver := app.NewDepositCandidateResolver(
		solana.New(cfg, m.deps.Clock),
		chain.SolanaAddress(cfg.Solana.USDCMint),
		m.owners,
		app.NewCreditDepositHandler(m.deps.UoW, m.deps.Bus),
		m.deps.IDs,
	)
	return []bus.Consumer{{
		Durable: "funding",
		Handlers: []bus.HandlerSpec{
			bus.HandleFetched("funding.resolve_deposit_candidate", resolver.Fetch, resolver.Apply),
		},
	}}
}

func (m *Module) Pollers() []poller.Poller {
	cfg := m.deps.Config
	return []poller.Poller{app.NewDepositPoller(m.deps.Pool, m.deps.UoW, m.deps.IDs, m.deps.Clock,
		identity.New(m.deps).Queries(), solana.New(cfg, m.deps.Clock), chain.SolanaAddress(cfg.Solana.USDCMint),
		cfg.Funding.DepositPollInterval, app.NewRPCLimiter(cfg.Funding.DepositRPCRate), m.deps.Bus)}
}

func (m *Module) Balances() port.Balances {
	if m.balances != nil {
		return m.balances
	}
	cfg := m.deps.Config
	m.balances = adapters.NewBalances(
		app.WalletReader{Reader: identity.New(m.deps).Queries()},
		func() adapters.TokenBalances { return solana.New(cfg, m.deps.Clock) },
		app.NoFundTransfers{},
		m.deps.Clock,
		chain.Mint{Address: chain.SolanaAddress(cfg.Solana.USDCMint), Decimals: 6},
	)
	return m.balances
}

func (*Module) Pauses() port.Pauses { return adapters.UnwiredPauses{} }

func (*Module) SignatureOwner() chain.SignatureOwnerFunc {
	return adapters.UnwiredSignatureOwner{}.OwnsSignature
}

type (
	Balances       = port.Balances
	Balance        = port.Balance
	Pauses         = port.Pauses
	Pause          = port.Pause
	PausedSet      = port.PausedSet
	PauseReason    = domain.PauseReason
	SignatureOwner = port.SignatureOwner
)

const (
	PauseReasonExternalDeposit = domain.PauseReasonExternalDeposit
	PauseReasonOps             = domain.PauseReasonOps
)
