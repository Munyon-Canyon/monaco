//go:build faultpoints

package treasury_test

import (
	"math/rand/v2"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seedChaosHoldings(t *testing.T, h testkit.Harness, cabal ids.CabalID, asset string, units, cost uint64) {
	t.Helper()
	if _, err := h.Pool.Exec(t.Context(), `INSERT INTO cabal_positions
		(cabal_id, asset, units, cost_basis_micros, updated_at) VALUES ($1, $2, $3, $4, now())`,
		cabal.UUID(), asset, units, cost); err != nil {
		t.Fatal(err)
	}
}

func seedCausalChains(t *testing.T, h testkit.Harness) {
	t.Helper()
	for _, cabal := range []ids.CabalID{chaosCabal(0), chaosCabal(1), chaosCabal(3)} {
		seedChaosHoldings(t, h, cabal, usdcMint, 100_000_000, 100_000_000)
	}
	seedChaosHoldings(t, h, chaosCabal(2), baseAAPL, 400_000_000, 10_000_000)
}

func causalChains() []events.Event {
	solana, bridged, sold, other := chaosCabal(0), chaosCabal(1), chaosCabal(2), chaosCabal(3)
	out := bridgeRun{cabal: bridged, swap: chaosSwap(2), step: "bridge_out"}
	sellSwap := chaosSwap(3)
	sale := evmConfirmed(sold, sellSwap, 0)
	sale.Action, sale.InMint, sale.InAmount, sale.OutMint, sale.OutAmount = "sell", baseAAPL, 400_000_000, baseCash, 9_900_000
	sale.USDCMicros, sale.FeeMicros = money.MicrosFromUint64(9_900_000), money.MicrosFromUint64(0)
	back := bridgeRun{cabal: sold, swap: sellSwap, step: "bridge_back"}
	return []events.Event{
		buy(solana, chaosSwap(0), 60_000_000, 3_000_000, 0),
		out.event("pending", "", 10_000_000, 0),
		sale,
		buy(other, chaosSwap(4), 40_000_000, 2_000_000, 0),
		out.event("submitted", "privy-out", 10_000_000, 0),
		back.event("pending", "", 9_900_000, 0),
		sell(solana, chaosSwap(1), 3_000_000, 61_000_000),
		out.event("succeeded", "privy-out", 10_000_000, 7_500),
		back.event("submitted", "privy-back", 9_900_000, 0),
		sell(other, chaosSwap(5), 2_000_000, 41_000_000),
		evmConfirmed(bridged, out.swap, 400_000_000),
		back.event("succeeded", "privy-back", 9_900_000, 7_000),
	}
}

func TestTradesConsumer_causalChainsFromZeroPositionsReachTheInOrderLedgerWhateverTheDeliveryOrder(t *testing.T) {
	t.Parallel()
	chain := causalChains()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedCausalChains(t, h)
		deps := module.Deps{Config: testkit.Config(), Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}
		for _, c := range treasury.New(deps).Consumers() {
			if c.Durable == "treasury_trades" {
				return c
			}
		}
		t.Fatal("treasury_trades is not registered")
		return bus.Consumer{}
	}, func(_ *rand.Rand, i int) events.Event { return chain[i] })
}
