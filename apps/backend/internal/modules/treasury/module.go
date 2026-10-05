package treasury

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/tools/ops/replay"
)

type Module struct {
	deps    module.Deps
	members app.Members
	users   app.Users
	cabals  app.CabalViews
	pauses  app.CashOutPauses
	fund    fundDeps
}

type fundDeps struct {
	cabals  app.FundCabals
	wallets app.FundWallets
	funding fundingProvider
}

type fundingProvider interface {
	Balances() fundingport.Balances
	PausesIn(tx db.Tx) fundingport.Pauses
}

type (
	Queries        = port.Queries
	Position       = port.Position
	Stake          = port.Stake
	CabalPositions = port.CabalPositions
	MemberStake    = port.MemberStake
	SignatureOwner = port.SignatureOwner
	WalletLedger   = port.WalletLedger
)

func New(d module.Deps) *Module {
	return &Module{
		deps: d, members: app.UnwiredReads{}, users: app.UnwiredReads{}, cabals: app.UnwiredReads{},
		pauses: app.CashOutPauseFunc(func(context.Context, ids.CabalID) (app.CashOutPause, error) {
			return app.CashOutPause{}, errs.New(
				errs.CodeUpstreamUnavailable,
				"treasury.UnwiredCashOutPauses",
			)
		}),
	}
}

func (*Module) Name() string { return "treasury" }

func (m *Module) Wire(set module.Set) {
	for _, mod := range set {
		switch provider := mod.(type) {
		case interface{ Queries() cabalport.Queries }:
			m.members = provider.Queries()
			m.cabals = provider.Queries()
			m.fund.cabals = provider.Queries()
		case interface{ Queries() identityport.Queries }:
			m.users = provider.Queries()
			m.fund.wallets = provider.Queries()
		}
		if provider, ok := mod.(interface{ Pauses() fundingport.Pauses }); ok {
			pauses := provider.Pauses()
			m.pauses = app.CashOutPauseFunc(func(ctx context.Context, cabal ids.CabalID) (app.CashOutPause, error) {
				pause, err := pauses.IsPaused(ctx, cabal)
				reasons := make([]string, len(pause.Reasons))
				for i, reason := range pause.Reasons {
					reasons[i] = string(reason)
				}
				return app.CashOutPause{Paused: pause.Paused, Reasons: reasons, Since: pause.Since}, err
			})
		}
		if provider, ok := mod.(fundingProvider); ok {
			m.fund.funding = provider
		}
	}
}

func (m *Module) Mount(r api.Mount) {
	names := catalogNames{Catalog: market.New(m.deps).Catalog()}
	treasuryapi.Mount(adapters.HTTP{
		Reads:     app.NewActivityReads(m.deps.Pool, m.members, m.users, names),
		UserTxns:  app.NewUserTxnReads(m.deps.Pool, m.cabals, usdc(m.deps.Config)),
		FundReads: m.deps.Pool,
		Fund:      m.fundCabalHandler(adapters.NewTransfers(m.deps.Config, m.deps.Clock)),
		CashOut: app.NewCashOutHandler(
			m.deps.UoW,
			m.ledger(),
			m.reads(),
			m.pauses,
			m.deps.Clock,
			m.deps.IDs,
			m.deps.Pool,
		),
	}, r)
}

func (m *Module) fundCabalHandler(transfers app.FundTransfers) *app.FundCabalHandler {
	if m.fund.funding == nil {
		return nil
	}
	return app.NewFundCabalHandler(app.FundCabalDeps{
		UoW: m.deps.UoW, IDs: m.deps.IDs, Clock: m.deps.Clock, Cabals: m.fund.cabals, Wallets: m.fund.wallets,
		Balances: m.fund.funding.Balances(), Pauses: m.fund.funding.PausesIn, Pot: m.reads(), Transfers: transfers,
		USDC: chain.Mint{Address: chain.SolanaAddress(m.deps.Config.Solana.USDCMint), Decimals: usdcDecimals},
	})
}

func (m *Module) Consumers() []bus.Consumer {
	activity := adapters.Activity{Hints: m.deps.Bus}
	userLedger := adapters.UserLedger{
		Ledger: m.ledger(),
		IDs:    m.deps.IDs,
		USDC:   usdc(m.deps.Config),
		Hints:  m.deps.Bus,
	}
	return []bus.Consumer{
		{
			Durable: "treasury_trades",
			Handlers: []bus.HandlerSpec{
				bus.Handle("treasury.trades", adapters.Trades{Ledger: m.ledger()}.Handle),
			},
		},
		{
			Durable: "treasury_activity",
			Handlers: []bus.HandlerSpec{
				bus.Handle("treasury.activity.submitted", activity.Submitted),
				bus.Handle("treasury.activity.confirmed", activity.Confirmed),
				bus.Handle("treasury.activity.failed", activity.Failed),
				bus.Handle("treasury.activity.fund_submitted", activity.FundSubmitted),
				bus.Handle("treasury.activity.funded", activity.Funded),
				bus.Handle("treasury.activity.fund_failed", activity.FundFailed),
			},
		},
		{
			Durable: "treasury_user_ledger",
			Handlers: []bus.HandlerSpec{
				bus.Handle("treasury.user_ledger", userLedger.Handle),
				bus.Handle("treasury.withdrawal_ledger", userLedger.Withdrawal),
			},
		},
	}
}

func (m *Module) Pollers() []poller.Poller {
	cfg := m.deps.Config
	return []poller.Poller{adapters.FundPoller{Settler: app.NewFundSettler(app.FundSettlerDeps{
		Reads: m.deps.Pool, UoW: m.deps.UoW, IDs: m.deps.IDs, Clock: m.deps.Clock,
		Chain: adapters.NewStatuses(cfg, m.deps.Clock), Transfers: adapters.NewTransfers(cfg, m.deps.Clock),
		Pot: m.reads(), Ledger: m.ledger(), USDC: usdc(cfg), Hints: m.deps.Bus,
	})}}
}

func (m *Module) Queries() port.Queries {
	return m.reads()
}

func (m *Module) SignatureOwner() *adapters.Queries { return m.reads() }

func (m *Module) WalletLedger() *adapters.Queries { return m.reads() }

func (m *Module) FundOutflows() adapters.FundOutflows { return adapters.NewFundOutflows(m.deps.Pool) }

func (m *Module) reads() *adapters.Queries {
	marketModule := market.New(m.deps)
	return adapters.NewQueries(
		m.deps.Pool,
		marketResolver(marketModule.Catalog()),
		marketPrices(marketModule.Prices()),
		m.deps.Clock,
		chain.SolanaAddress(m.deps.Config.Solana.USDCMint),
	)
}

func marketResolver(catalog market.Catalog) app.MintResolver {
	return func(ctx context.Context, address chain.SolanaAddress) (app.Asset, error) {
		mint, err := market.ParseMint(string(address))
		if err != nil {
			return app.Asset{}, err
		}
		asset, err := catalog.AssetByMint(ctx, mint)
		if err != nil {
			return app.Asset{}, err
		}
		return app.Asset{
			ID: asset.ID.UUID(), Decimals: asset.Decimals, ChainChecked: asset.ChainChecked,
			UIMultiplierNum: asset.UIMultiplier.Num, UIMultiplierDen: asset.UIMultiplier.Den,
			NextUIMultiplierNum: asset.NextUIMultiplier.To.Num,
			NextUIMultiplierDen: asset.NextUIMultiplier.To.Den,
			NextUIMultiplierAt:  asset.NextUIMultiplier.At,
		}, nil
	}
}

func marketPrices(reader market.Prices) app.PriceReader {
	return func(ctx context.Context) (map[uuid.UUID]app.Price, error) {
		prices, err := reader.LatestPrices(ctx)
		if err != nil {
			return nil, err
		}
		out := make(map[uuid.UUID]app.Price, len(prices))
		for id, price := range prices {
			out[id.UUID()] = app.Price{Micros: price.Micros, ObservedAt: price.ObservedAt}
		}
		return out, nil
	}
}

func (m *Module) ledger() app.Ledger {
	return app.NewLedger(chain.SolanaAddress(m.deps.Config.Solana.USDCMint), m.deps.Clock)
}

const usdcDecimals = 6

func usdc(cfg config.Config) domain.Asset {
	return domain.MintAsset(chain.SolanaAddress(cfg.Solana.USDCMint))
}

func LedgerCheck(cfg config.Config) replay.LedgerCheck {
	return replay.LedgerCheck{
		Name: "treasury",
		Tables: []string{
			"cabal_txns", "cabal_txn_entries", "user_txns", "user_txn_entries", "cabal_positions", "user_positions",
		},
		Check: adapters.CheckLedger(usdc(cfg), map[events.Type]adapters.BalanceRule{
			events.TypeDepositCredited:     adapters.DepositCreditedBalances(usdc(cfg)),
			events.TypeWithdrawalConfirmed: adapters.WithdrawalConfirmedBalances(usdc(cfg)),
		}),
	}
}
