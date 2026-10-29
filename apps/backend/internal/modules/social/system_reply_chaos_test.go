//go:build faultpoints

package social_test

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func systemReplyEvent(_ *rand.Rand, i int) events.Event {
	switch i {
	case 0:
		return events.TradeConfirmed{
			V:           1,
			SwapID:      uuid.NewSHA1(uuid.Nil, []byte("system-swap")),
			CabalID:     chaosCabal(2),
			Source:      events.TradeSource{Kind: "proposal", ID: resultProposal(0)},
			Action:      "buy",
			Symbol:      "AAPLx",
			OutMint:     marketfake.AAPLx().Mint.Address(),
			OutAmount:   2,
			ConfirmedAt: time.Date(2026, 10, 22, 9, 41, 0, 0, time.UTC),
		}
	case 1:
		return events.ProposalFailed{V: 1, ProposalID: resultProposal(1), CabalID: chaosCabal(2)}
	case 2:
		return events.ProposalExpired{V: 1, ProposalID: resultProposal(2), CabalID: chaosCabal(2)}
	default:
		return events.TradeFailed{
			V: 1, SwapID: uuid.NewSHA1(uuid.Nil, []byte("system-failed")), CabalID: chaosCabal(2),
			Source: events.TradeSource{Kind: "proposal", ID: resultProposal(0)}, Action: "buy", Symbol: "AAPLx",
		}
	}
}

func TestSystemReply_Chaos_RedeliveryAndReorderingWriteEachOutcomeOnce(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedResultProposals(t, h)
		held := fakes.NewTreasury()
		held.SetPositions(ids.CabalIDFrom(chaosCabal(2)), []treasury.Position{
			{Mint: marketfake.AAPLx().Mint.Address(), Units: money.NewBaseUnits(5, 0)},
		})
		deps := module.Deps{Pool: h.Pool, UoW: db.New(h.Pool, h.IDs, h.Clock), IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}
		mod := social.New(deps, social.WithTallies(chaosTallies()), social.WithCabals(chaosNames()),
			social.WithAssets(marketfake.NewCatalog(marketfake.AAPLx())),
			social.WithHoldings(social.PositionHoldings{Positions: held, Swaps: held}))
		for _, c := range mod.Consumers() {
			if c.Durable == "social_thread" {
				return c
			}
		}
		t.Fatal("social_thread is not registered")
		return bus.Consumer{}
	}, systemReplyEvent)
}
