package funding

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/fundingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps     module.Deps
	balances port.Balances
}

func New(d module.Deps) *Module { return &Module{deps: d} }

func (*Module) Name() string { return "funding" }

func (m *Module) Mount(r api.Mount) {
	cfg := m.deps.Config
	wallets := app.WalletReader{Reader: identity.New(m.deps).Queries()}
	fundingapi.Mount(adapters.HTTP{
		Balances: m.Balances(), Wallets: wallets,
		Create:   app.NewCreateOnrampSessionHandler(m.deps.UoW, m.deps.Clock, cfg.FundPageURL()),
		Exchange: app.NewExchangeOnrampTokenHandler(m.deps.UoW, m.deps.Clock, wallets, cfg.Solana.USDCMint),
		IDs:      m.deps.IDs,
	}, r)
}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{}
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

func (m *Module) Pauses() port.Pauses { return adapters.NewPauses(m.deps.Pool) }

func (*Module) PausesIn(tx db.Tx) port.Pauses { return adapters.NewPauses(tx.Queries()) }

func (m *Module) PauseFromOps(ctx context.Context, cabalID *ids.CabalID, note string) (uuid.UUID, error) {
	pause := app.NewPauseCabalHandler(m.deps.UoW, m.deps.IDs, m.deps.Clock, m.deps.Bus)
	return pause.Handle(opsActor(ctx), app.PauseCabal{CabalID: cabalID, Reason: domain.PauseReasonOps, Note: note})
}

func (m *Module) ResumeFromOps(ctx context.Context, cabalID *ids.CabalID) error {
	resume := app.NewResumeCabalHandler(m.deps.UoW, m.deps.Clock, m.deps.Bus)
	return resume.Handle(opsActor(ctx), app.ResumeCabal{CabalID: cabalID})
}

func opsActor(ctx context.Context) context.Context {
	return auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: "monacoctl"})
}

func (*Module) SignatureOwner() adapters.UnwiredSignatureOwner {
	return adapters.UnwiredSignatureOwner{}
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
