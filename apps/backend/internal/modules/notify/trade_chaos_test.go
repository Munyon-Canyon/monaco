//go:build faultpoints

package notify_test

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func chaosTrade(i int) events.Event {
	round := i / 4
	amounts := []uint64{1_250_000, 12_500_000, 9_999, 50_000_000}
	named := func(prefix string) uuid.UUID {
		return uuid.NewSHA1(uuid.NameSpaceOID, []byte("notify-chaos-"+prefix+"-"+strconv.Itoa(i)))
	}
	confirmed := events.TradeConfirmed{
		V: 1, SwapID: named("swap"), CabalID: chaosCabal(round % 2),
		Source:          events.TradeSource{Kind: proposalKind, ID: named("proposal")},
		SourceBatchSize: 1, Action: "buy", Symbol: []string{"AAPLx", "TSLAx", "ZZZZx"}[round%3],
		USDCMicros: money.MicrosFromUint64(amounts[i%len(amounts)]),
	}
	switch i % 4 {
	case 1:
		confirmed.Action = "sell"
	case 2:
		return events.TradeFailed{
			V: 1, SwapID: confirmed.SwapID, CabalID: confirmed.CabalID, Source: confirmed.Source, SourceBatchSize: 1,
			Action: []string{"buy", "sell"}[round%2], Symbol: confirmed.Symbol, FailureCode: "jupiter_failed",
		}
	case 3:
		confirmed.Action, confirmed.Source.Kind = "sell", cashOutKind
	}
	return confirmed
}

func TestNotify_TradeKinds_ConvergeUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedChaosUsers(t, h)
		deps := module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, UoW: db.New(h.Pool, h.IDs, h.Clock)}
		m := notify.New(deps, notify.WithSender(&testkit.FakeSender{}), notify.WithCabals(chaosCabals()),
			notify.WithAssets(marketfake.NewCatalog(marketfake.Fixtures()...)))
		return m.Consumers()[0]
	}, func(_ *rand.Rand, i int) events.Event { return chaosTrade(i) })
}
