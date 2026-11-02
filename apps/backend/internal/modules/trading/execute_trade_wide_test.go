package trading_test

import (
	"math/big"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func tokens(whole int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(whole), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
}

func wideUnits(t *testing.T, whole int64) money.Units {
	t.Helper()
	u, err := money.UnitsFromBig(tokens(whole))
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (e *routeEnv) planWide(t *testing.T, whole int64) {
	t.Helper()
	floor := new(big.Int).Mul(tokens(whole), big.NewInt(99))
	floor.Quo(floor, big.NewInt(100))
	floorUnits, err := money.UnitsFromBig(floor)
	if err != nil {
		t.Fatal(err)
	}
	e.evm.plan.OutAmount, e.evm.plan.MinOut = wideUnits(t, whole), floorUnits
	e.requoteWith(multichain.Quote{GrossOutput: tokens(whole)})
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: wideUnits(t, whole)}}
}

func TestExecuteTrade_anEVMBuyOfManyTokensSettlesAndConfirmsTheExactFill(t *testing.T) {
	t.Parallel()
	for name, whole := range map[string]int64{"5 tokens": 5, "10 tokens": 10, "25 tokens": 25, "100 tokens": 100} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			e.privy.script(multichain.ActionSucceeded)
			e.planWide(t, whole)
			e.route.Ranked[0].GrossOutput = tokens(whole)
			cmd := e.engineEnv.buy()
			cmd.Route = e.route
			d := e.delivery(t, cmd)

			if err := e.tradeHandler().Handle(actorContext(t.Context()), d, cmd, nil); err != nil {
				t.Fatal(err)
			}

			swaps := e.swapsOf(t, cmd.ProposalID)
			if len(swaps) != 1 || e.deliveryCode(t, d) != "ok" {
				t.Fatalf(
					"%d swaps, delivery %s, want the %d-token buy settled",
					len(swaps),
					e.deliveryCode(t, d),
					whole,
				)
			}
			confirmed := e.payload(t, swaps[0], "trade.confirmed")
			if confirmed["out_amount"] != tokens(whole).String() {
				t.Fatalf("trade.confirmed = %v, want out_amount %s", confirmed, tokens(whole))
			}
			if len(e.evm.specs) != 1 {
				t.Fatalf("built %d swaps, want one", len(e.evm.specs))
			}
		})
	}
}

func TestExecuteTrade_anEVMBuyRequotedPastInt64IsNotReadAsPriceMoved(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.planWide(t, 25)
	cmd := e.engineEnv.buy()
	cmd.Route = e.route

	if err := e.tradeHandler().Handle(actorContext(t.Context()), e.delivery(t, cmd), cmd, nil); err != nil {
		t.Fatal(err)
	}

	swaps := e.swapsOf(t, cmd.ProposalID)
	if len(swaps) != 1 || e.failure(t, swaps[0]) != "" {
		t.Fatalf("%d swaps, failure %q, want a settled 25-token buy", len(swaps), e.failure(t, swaps[0]))
	}
	if len(e.evm.specs) != 1 || e.evm.specs[0].MinOut.IsZero() {
		t.Fatalf("specs = %+v, want one swap with a floor above zero", e.evm.specs)
	}
}

func TestExecuteTrade_anEVMSaleOf12TokensSellsExactlyTheProposedUnits(t *testing.T) {
	t.Parallel()
	units := uint64(12) * 1_000_000_000_000_000_000
	e := newRouteEnv(t)
	e.selling()
	e.privy.script(multichain.ActionSucceeded)
	cmd := e.sell(units)
	cmd.Route = e.route

	code := e.passedSale(t, e.sellHandler(e.heldOnBase(units)), cmd, chain.NetworkBase)

	swaps := e.swapsOf(t, cmd.ProposalID)
	if code != "ok" || len(swaps) != 1 {
		t.Fatalf("delivery %s with %d swaps, want one routed 12-token sale", code, len(swaps))
	}
	e.wantSold(t, swaps[0], units)
}

func TestExecuteTrade_anEVMBuyRequotedPastTheColumnWidthFailsPriceMoved(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.requoteWith(multichain.Quote{GrossOutput: new(big.Int).Lsh(big.NewInt(1), 300)})
	cmd := e.engineEnv.buy()
	cmd.Route = e.route
	d := e.delivery(t, cmd)

	if err := e.tradeHandler().Handle(actorContext(t.Context()), d, cmd, nil); err != nil {
		t.Fatal(err)
	}

	swaps := e.swapsOf(t, cmd.ProposalID)
	if len(swaps) != 1 || e.failure(t, swaps[0]) != "price_moved" || len(e.evm.specs) != 0 {
		t.Fatalf("%d swaps built %d, want a price_moved failure before any swap", len(swaps), len(e.evm.specs))
	}
}
