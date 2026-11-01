package trading_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

func tokens(whole int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(whole), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
}

func TestExecuteTrade_anEVMBuyQuotedPastInt64IsRefusedBeforeTheBridge(t *testing.T) {
	t.Parallel()
	for name, whole := range map[string]int64{"10 tokens": 10, "25 tokens": 25} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			e.privy.script(multichain.ActionSucceeded)
			e.route.Ranked[0].GrossOutput = tokens(whole)
			cmd := e.engineEnv.buy()
			cmd.Route = e.route
			d := e.delivery(t, cmd)

			if err := e.tradeHandler().Handle(actorContext(t.Context()), d, cmd, nil); err != nil {
				t.Fatal(err)
			}

			e.wantPinRefused(t, cmd, e.deliveryCode(t, d), errs.CodeTradeTooLarge)
			if len(e.privy.calls()) != 0 || len(e.evm.specs) != 0 {
				t.Fatalf(
					"%d transfers and %d swaps built, want none before a refusal",
					len(e.privy.calls()),
					len(e.evm.specs),
				)
			}
		})
	}
}

func TestExecuteTrade_anEVMBuyQuotedWithinInt64StillSettles(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	e.quote(usdcToken(), aaplxToken(), 0, false)
	e.route.Ranked[0].GrossOutput = tokens(5)
	cmd := e.engineEnv.buy()
	cmd.Route = e.route
	d := e.delivery(t, cmd)

	if err := e.tradeHandler().Handle(actorContext(t.Context()), d, cmd, nil); err != nil {
		t.Fatal(err)
	}

	if swaps := e.swapsOf(t, cmd.ProposalID); len(swaps) != 1 || e.deliveryCode(t, d) != "ok" {
		t.Fatalf("%d swaps, delivery %s, want the 5-token buy settled", len(swaps), e.deliveryCode(t, d))
	}
}

func TestExecuteTrade_anEVMSaleOfMoreThanInt64IsRefusedBeforeTheSwap(t *testing.T) {
	t.Parallel()
	const units = uint64(math.MaxInt64) + 1
	e := newRouteEnv(t)
	e.selling()
	e.privy.script(multichain.ActionSucceeded)
	cmd := e.sell(units)
	cmd.Route = e.route

	code := e.passedSale(t, e.sellHandler(e.heldOnBase(units)), cmd, chain.NetworkBase)

	e.wantPinRefused(t, cmd, code, errs.CodeTradeTooLarge)
	if len(e.privy.calls()) != 0 || len(e.evm.specs) != 0 {
		t.Fatalf("%d transfers and %d swaps built, want none", len(e.privy.calls()), len(e.evm.specs))
	}
}
