package governance_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const (
	potMicros  = 100_000_000
	heldUnits  = 500_000_000
	quoteUnits = 21_000_000
)

type liveCabals struct{ current atomic.Pointer[fakes.Cabal] }

func (c *liveCabals) IsMember(ctx context.Context, id ids.CabalID, user ids.UserID) (bool, error) {
	return c.current.Load().IsMember(ctx, id, user)
}

func (c *liveCabals) Rules(ctx context.Context, id ids.CabalID) (cabal.Rules, error) {
	return c.current.Load().Rules(ctx, id)
}

func (c *liveCabals) TreasuryWallet(ctx context.Context, id ids.CabalID) (cabal.TreasuryWallet, error) {
	return c.current.Load().TreasuryWallet(ctx, id)
}

func (c *liveCabals) VoterSet(ctx context.Context, id ids.CabalID) ([]ids.UserID, error) {
	return c.current.Load().VoterSet(ctx, id)
}

type tradeWorld struct {
	cabal    ids.CabalID
	members  []ids.UserID
	seed     fakes.CabalSeed
	rows     []fakes.CabalMember
	cabals   *liveCabals
	catalog  *marketfake.CatalogFake
	routes   *marketfake.RoutesFake
	treasury *fakes.Treasury
	chain    *chainfake.Ledger
}

const treasuryAddress = chain.SolanaAddress("treasury-wallet")

func newTradeWorld(t *testing.T) *tradeWorld {
	t.Helper()
	g := testkit.NewIDs(testkit.RandSeed(t))
	w := &tradeWorld{
		cabal: ids.CabalIDFrom(g.NewV7()), cabals: &liveCabals{}, routes: &marketfake.RoutesFake{},
		catalog: marketfake.NewCatalog(marketfake.Fixtures()...), treasury: fakes.NewTreasury(),
		chain: chainfake.NewLedger(clock.Real{}),
	}
	for range 3 {
		w.members = append(w.members, ids.NewUserID(g))
	}
	w.seed = fakes.CabalSeed{
		View: cabal.View{ID: w.cabal, Name: "Friends pot", CreatorID: w.members[0], Status: cabal.StatusActive},
		Rules: cabal.Rules{
			JoinMode: cabal.JoinRequest, VoterMode: cabal.VotersAll, Threshold: cabal.ThresholdMajority,
			ProposalExpiry: 24 * time.Hour, SlippageBps: 100,
		},
		Wallet: cabal.TreasuryWallet{CabalID: w.cabal, PrivyWalletID: "treasury", Address: treasuryAddress},
	}
	for _, m := range w.members {
		w.join(m, true)
	}
	aapl := marketfake.AAPLx()
	w.routes.Ok(aapl.ID, market.RouteCheck{OutAmount: money.NewBaseUnits(quoteUnits, aapl.Decimals)})
	w.routes.Untradable(marketfake.JPSTx().ID)
	w.routes.NoRoute(marketfake.TSLAx().ID)
	w.treasury.SetPotValue(w.cabal, money.MicrosFromUint64(potMicros))
	w.treasury.SetPositions(w.cabal, []treasury.Position{
		{Mint: marketfake.TSLAx().Mint.Address(), Units: money.NewBaseUnits(heldUnits*2, aapl.Decimals)},
		{Mint: aapl.Mint.Address(), Units: money.NewBaseUnits(heldUnits, aapl.Decimals)},
	})
	w.holdOnChain(heldUnits)
	return w
}

func (w *tradeWorld) holdOnChain(units uint64) {
	aapl := marketfake.AAPLx()
	w.chain.SetTokens(treasuryAddress, chain.Mint{Address: aapl.Mint.Address(), Decimals: aapl.Decimals}, units)
}

func (w *tradeWorld) join(user ids.UserID, canVote bool) {
	w.rows = append(w.rows, fakes.CabalMember{CabalID: w.cabal, Member: cabal.MemberView{
		UserID: user, Role: cabal.RoleMember, CanVote: canVote, JoinedAt: clock.Real{}.Now(),
	}})
	w.cabals.current.Store(fakes.NewCabal([]fakes.CabalSeed{w.seed}, w.rows))
}

func (w *tradeWorld) ports() app.TradePorts {
	return app.TradePorts{
		Cabals: w.cabals, Assets: w.catalog, Routes: w.routes, Treasury: w.treasury, Balances: w.chain,
	}
}
