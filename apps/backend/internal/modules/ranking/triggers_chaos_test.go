//go:build faultpoints

package ranking_test

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestTriggers_convergeUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		return rankingConsumer(t, h, "ranking_triggers")
	}, func(rng *rand.Rand, i int) events.Event {
		cabal := uuid.NewSHA1(uuid.Nil, []byte("cabal"+strconv.Itoa(i)+strconv.FormatUint(rng.Uint64(), 10)))
		switch i % 6 {
		case 0:
			return events.TradeConfirmed{V: 1, CabalID: cabal}
		case 1:
			return events.Funded{V: 1, CabalID: cabal}
		case 2:
			return events.CashOutCompleted{V: 1, CabalID: cabal}
		case 3:
			return events.CashOutStarted{V: 1, CabalID: cabal}
		case 4:
			return events.CashOutPartial{V: 1, CabalID: cabal}
		}
		return events.CashOutFailed{V: 1, CabalID: cabal}
	})
}
