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
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func chaosResult(i int) events.Event {
	round := i / 6
	named := func(prefix string) uuid.UUID {
		return uuid.NewSHA1(uuid.NameSpaceOID, []byte("notify-chaos-"+prefix+"-"+strconv.Itoa(i)))
	}
	cabalID, symbol := chaosCabal(round%2), []string{"AAPLx", "TSLAx", "ZZZZx"}[round%3]
	source := events.TradeSource{Kind: proposalKind, ID: named("proposal")}
	switch i % 6 {
	case 1:
		return events.TradeConfirmed{
			V: 1, SwapID: named("swap"), CabalID: cabalID, Source: source, SourceBatchSize: 1, Action: "sell",
			Symbol: "AAPLx", InAmount: 220_000_000, OutAmount: 1_000_000_000,
			USDCMicros: money.MicrosFromUint64(1_000_000_000),
		}
	case 2:
		return events.TradeFailed{
			V: 1, SwapID: named("swap"), CabalID: cabalID, Source: source, SourceBatchSize: 1,
			Action: "buy", Symbol: symbol, FailureCode: "jupiter_failed",
		}
	case 3:
		return events.TradeBlocked{
			V:               1,
			CabalID:         cabalID,
			Source:          source,
			SourceBatchSize: 1,
			Action:          "sell",
			Symbol:          symbol,
			Code:            "no_route",
		}
	case 4:
		return events.ProposalFailed{V: 1, ProposalID: source.ID, CabalID: cabalID}
	case 5:
		return events.ProposalExpired{V: 1, ProposalID: source.ID, CabalID: cabalID}
	}
	return events.TradeConfirmed{
		V: 1, SwapID: named("swap"), CabalID: cabalID, Source: source, SourceBatchSize: 1, Action: "buy",
		Symbol: "TSLAx", InAmount: 12_500_000, OutAmount: 5_000_000, USDCMicros: money.MicrosFromUint64(12_500_000),
	}
}

func TestNotify_ResultKinds_ConvergeUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedChaosUsers(t, h)
		deps := module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, UoW: db.New(h.Pool, h.IDs, h.Clock)}
		user := func(i int) ids.UserID { return ids.UserIDFrom(chaosRecipient(i)) }
		ballots := everyProposal{yesBallot(user(3)), noBallot(user(2)), yesBallot(user(0)), eligibleBallot(user(1))}
		m := notify.New(deps, notify.WithSender(&testkit.FakeSender{}), notify.WithCabals(chaosCabals()),
			notify.WithBallots(ballots), notify.WithProposalHeads(everyHead{}),
			notify.WithAssets(marketfake.NewCatalog(marketfake.Fixtures()...)))
		return m.Consumers()[0]
	}, func(_ *rand.Rand, i int) events.Event { return chaosResult(i) })
}
