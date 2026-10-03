//go:build faultpoints

package treasury_test

import (
	"math/rand/v2"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestActivityConsumer_convergesUnderDuplicatesAndReordering(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		for _, c := range treasury.New(module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}).Consumers() {
			if c.Durable == "treasury_activity" {
				return c
			}
		}
		t.Fatal("treasury_activity is not registered")
		return bus.Consumer{}
	}, func(_ *rand.Rand, i int) events.Event {
		n := i / 4
		cabal, swap := chaosCabal(n), chaosSwap(n)
		stock := chain.SolanaAddress(aapl)
		switch {
		case i%4 < 2:
			return submitted(cabal, swap, "buy", usdcMint, stock, 7_000_000)
		case n%2 == 0:
			return buy(cabal, swap, 7_000_000, 300_000, 0)
		default:
			return failed(cabal, swap, "buy", usdcMint, 7_000_000)
		}
	})
}
