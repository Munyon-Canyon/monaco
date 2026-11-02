package governance_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	wideTokenDecimals = 18
	unitsPerSaleMicro = 5_000_000_000
)

type ledgerEVMTreasury struct {
	*evmTreasury
	reads app.EVMHoldings
}

func (l ledgerEVMTreasury) EVMPositions(ctx context.Context, id ids.CabalID) ([]treasury.ListingPosition, error) {
	return l.reads.EVMPositions(ctx, id)
}

type ratioQuoter struct{ unitsPerMicro uint64 }

func (q ratioQuoter) Quote(
	_ context.Context, l multichain.Listing, _ multichain.Side, micros money.Micros,
) (multichain.Quote, error) {
	gross := new(big.Int).Mul(new(big.Int).SetUint64(micros.Uint64()), new(big.Int).SetUint64(q.unitsPerMicro))
	return multichain.Quote{ListingID: l.ID, GrossOutput: gross}, nil
}

type wideFixture struct {
	h      *app.ProposeTradeHandler
	d      proposalDB
	twenty money.Units
}

func newWideFixture(t *testing.T, e *evmSellWorld) wideFixture {
	t.Helper()
	e.base.Decimals = wideTokenDecimals
	e.listings["base:"+baseToken] = e.base
	twenty, err := money.ParseUnits("20000000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	var h *app.ProposeTradeHandler
	s := scenario.New(t, scenario.WithModules(func(d module.Deps) module.Module {
		testkit.NewLedger(t, d.Pool).WithHoldingAsset(e.cabal, "base:"+baseToken, twenty)
		ports := e.ports()
		ports.Treasury = ledgerEVMTreasury{evmTreasury: e.evm, reads: treasury.New(d).Queries().(app.EVMHoldings)}
		ports.Listings, ports.Quoter = e.listings, ratioQuoter{unitsPerMicro: unitsPerSaleMicro}
		h = app.NewProposeTradeHandler(db.New(d.Pool, testkit.NewIDs(1), clock.Real{}), testkit.NewIDs(2),
			clock.Real{}, ports, app.NoHints{})
		return governance.New(d, governance.WithPorts(ports))
	}))
	return wideFixture{
		h: h, twenty: twenty, d: proposalDB{pool: s.DB(), q: sqlc.New(s.DB()), ids: testkit.NewIDs(3)},
	}
}

func (f wideFixture) sell(t *testing.T, e *evmSellWorld, tokens money.Units) (app.OpenedProposal, error) {
	t.Helper()
	ctx := observability.WithActor(t.Context(), "user:"+e.members[0].String())
	return f.h.Open(ctx, app.ProposeTrade{
		CabalID: e.cabal, ProposerID: e.members[0], Trade: domain.Trade{
			Kind: domain.KindSell, Symbol: "AAPLx", TokenAmount: tokens, Network: chain.NetworkBase,
		},
	})
}

func TestProposeTrade_aTwentyTokenEVMHoldingSellsAndReadsBackExactly(t *testing.T) {
	t.Parallel()
	e := newEVMSellWorld(t)
	f := newWideFixture(t, e)
	opened, err := f.sell(t, e, f.twenty)
	if err != nil {
		t.Fatalf("Open a sale of 20 tokens at 18 decimals = %v, want it opened", err)
	}
	if !opened.Draft.TokenAmount.Equal(f.twenty) || opened.TokenAmount != 0 ||
		opened.Draft.Decimals != wideTokenDecimals {
		t.Fatalf("opened = %s units, int64 view %d, %d decimals, want the exact 20 tokens and a zero int64 view",
			opened.Draft.TokenAmount, opened.TokenAmount, opened.Draft.Decimals)
	}
	if got := opened.Draft.QuoteOut.String(); got != "4000000000" {
		t.Fatalf("quote out = %s, want 4000 USDC", got)
	}
	f.d.wantWide(t, opened.Draft.ID.UUID(), e.cabal.UUID(), e.members[0].UUID(), f.twenty.String(), "4000000000")
}

func TestProposeTrade_aSellPastTheCabalsEVMHoldingIsStillShort(t *testing.T) {
	t.Parallel()
	e := newEVMSellWorld(t)
	f := newWideFixture(t, e)
	more, err := f.twenty.Add(money.UnitsFromUint64(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.sell(t, e, more); errs.CodeOf(err) != errs.CodeCabalSharesShort {
		t.Fatalf("Open one unit past the holding = %v, want cabal_shares_short", err)
	}
}
