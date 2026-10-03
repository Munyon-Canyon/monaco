//go:build faultpoints

package treasury_test

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func chaosCabal(i int) ids.CabalID {
	return ids.CabalIDFrom(uuid.NewSHA1(uuid.Nil, []byte("cabal-"+strconv.Itoa(i))))
}

func chaosSwap(i int) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte("swap-"+strconv.Itoa(i))) }

func seedChaosCabals(t *testing.T, h testkit.Harness, n int) {
	t.Helper()
	for i := range n {
		for _, asset := range []string{usdcMint, string(aapl)} {
			if _, err := h.Pool.Exec(t.Context(), `INSERT INTO cabal_positions
				(cabal_id, asset, units, cost_basis_micros, updated_at) VALUES ($1, $2, 1000000000, 1000000000, now())`,
				chaosCabal(i).UUID(), asset); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestTradesConsumer_duplicatedAndReorderedConfirmationsPostOneHeaderEach(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedChaosCabals(t, h, 6)
		for _, c := range treasury.New(module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}).Consumers() {
			if c.Durable == "treasury_trades" {
				return c
			}
		}
		t.Fatal("treasury_trades is not registered")
		return bus.Consumer{}
	}, func(_ *rand.Rand, i int) events.Event {
		n := i / 2
		k := [...]uint64{0, 1, 2, 3, 4, 5}[n]
		if n%2 == 0 {
			return buy(chaosCabal(n), chaosSwap(n), 10_000_000+k, 1_000_000+k, k)
		}
		return sell(chaosCabal(n), chaosSwap(n), 1_000_000+k, 10_000_000+k)
	})
}
