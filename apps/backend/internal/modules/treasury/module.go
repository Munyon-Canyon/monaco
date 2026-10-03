package treasury

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/tools/ops/replay"
)

type Module struct {
	deps    module.Deps
	members app.Members
	users   app.Users
}

type Queries = port.Queries

type Position = port.Position

type Stake = port.Stake

type CabalPositions = port.CabalPositions

type MemberStake = port.MemberStake

func New(d module.Deps) *Module {
	return &Module{deps: d, members: app.UnwiredReads{}, users: app.UnwiredReads{}}
}

func (*Module) Name() string { return "treasury" }

func (m *Module) Wire(set module.Set) {
	for _, mod := range set {
		switch provider := mod.(type) {
		case interface{ Queries() cabalport.Queries }:
			m.members = provider.Queries()
		case interface{ Queries() identityport.Queries }:
			m.users = provider.Queries()
		}
	}
}

func (m *Module) Routes(r *httpx.Routes) {
	names := catalogNames{Catalog: market.New(m.deps).Catalog()}
	r.TreasuryRoutes = adapters.HTTP{Reads: app.NewActivityReads(m.deps.Pool, m.members, m.users, names)}
}

func (m *Module) Consumers() []bus.Consumer {
	activity := adapters.Activity{Hints: m.deps.Bus}
	userLedger := adapters.UserLedger{Ledger: m.ledger(), IDs: m.deps.IDs, USDC: usdc(m.deps.Config)}
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
			},
		},
		{
			Durable: "treasury_user_ledger",
			Handlers: []bus.HandlerSpec{
				bus.Handle("treasury.user_ledger", userLedger.Handle),
			},
		},
	}
}

func (*Module) Pollers() []poller.Poller { return nil }

func (m *Module) Queries() port.Queries {
	marketModule := market.New(m.deps)
	return adapters.NewQueries(
		m.deps.Pool, marketResolver(marketModule.Catalog()), marketPrices(marketModule.Prices()), m.deps.Clock,
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
			events.TypeDepositCredited: adapters.DepositCreditedBalances(usdc(cfg)),
		}),
	}
}
