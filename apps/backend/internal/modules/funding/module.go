package funding

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
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
		Create:      app.NewCreateOnrampSessionHandler(m.deps.UoW, m.deps.Clock, cfg.FundPageURL()),
		Exchange:    app.NewExchangeOnrampTokenHandler(m.deps.UoW, m.deps.Clock, wallets, cfg.Solana.USDCMint),
		Report:      app.NewReportOnrampStatusHandler(m.deps.UoW, m.deps.Clock, m.deps.Bus),
		Reads:       m.deps.Pool,
		IDs:         m.deps.IDs,
		Withdrawals: app.NewWithdrawHandler(m.withdrawDeps(wallets)),
	}, r)
}

func (m *Module) withdrawDeps(wallets app.WalletReader) app.WithdrawDeps {
	return app.WithdrawDeps{
		UoW: m.deps.UoW, Balances: m.Balances(), Wallets: wallets, Hints: m.deps.Bus, Clock: m.deps.Clock,
		USDC: m.usdc(), Transfers: m.lazyTransfers(),
	}
}

func (m *Module) lazyTransfers() func() (app.Transfers, error) {
	return func() (app.Transfers, error) { return m.transfers() }
}

func (m *Module) transfers() (*relayer.Transfers, error) {
	signer, err := privy.New(m.deps.Config, m.deps.Clock)
	if err != nil {
		return nil, err
	}
	r, err := relayer.New(m.deps.Config, solana.New(m.deps.Config, m.deps.Clock))
	if err != nil {
		return nil, err
	}
	return relayer.NewTransfers(r, signer), nil
}

func (m *Module) usdc() chain.Mint {
	return chain.Mint{Address: chain.SolanaAddress(m.deps.Config.Solana.USDCMint), Decimals: 6}
}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{}
}

func (m *Module) Pollers() []poller.Poller {
	cfg := m.deps.Config
	withdrawals := app.NewWithdrawalPoller(app.WithdrawalPollerDeps{
		UoW: m.deps.UoW, Reads: m.deps.Pool, Clock: m.deps.Clock, Chain: solana.New(cfg, m.deps.Clock),
		Transfers: m.lazyTransfers(), Hints: m.deps.Bus,
	})
	return []poller.Poller{
		app.NewDepositPoller(m.deps.Pool, m.deps.UoW, m.deps.IDs, m.deps.Clock,
			identity.New(m.deps).Queries(), solana.New(cfg, m.deps.Clock), chain.SolanaAddress(cfg.Solana.USDCMint),
			cfg.Funding.DepositPollInterval, app.NewRPCLimiter(cfg.Funding.DepositRPCRate), m.deps.Bus,
		).SkipOwned(treasury.New(m.deps).SignatureOwner()),
		app.NewOnrampExpiryPoller(m.deps.UoW, m.deps.Clock),
		withdrawals,
	}
}

func (m *Module) Balances() port.Balances {
	if m.balances != nil {
		return m.balances
	}
	cfg := m.deps.Config
	m.balances = adapters.NewBalances(
		app.WalletReader{Reader: identity.New(m.deps).Queries()},
		func() adapters.TokenBalances { return solana.New(cfg, m.deps.Clock) },
		adapters.Outflows{
			Funds:       treasury.New(m.deps).FundOutflows(),
			Withdrawals: app.WithdrawalOutflows{Reads: m.deps.Pool},
		},
		m.deps.Clock,
		m.usdc(),
	)
	return m.balances
}

func (m *Module) Withdrawals() port.Withdrawals { return app.WithdrawalReads{Reads: m.deps.Pool} }

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

func (m *Module) SignatureOwner() adapters.BounceSignatures {
	return adapters.NewBounceSignatures(m.deps.Pool)
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
