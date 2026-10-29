//go:build faultpoints

package social_test

import (
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const resultProposals = 3

func resultProposal(i int) uuid.UUID {
	return uuid.NewSHA1(uuid.Nil, []byte("result-proposal"+strconv.Itoa(i)))
}

func seedResultProposals(t *testing.T, h testkit.Harness) {
	t.Helper()
	for i := range resultProposals {
		payload := feed.Payload{
			CabalName: "Alpha", ActorName: "Old " + strconv.Itoa(i), Symbol: "AAPLx", AssetName: "Apple",
			Action: feed.ActionBuy, Status: "open", AssetKind: feed.AssetKindEquity,
		}
		_, err := h.Pool.Exec(t.Context(), `INSERT INTO feed_objects
			(id, kind, ref_type, ref_id, cabal_id, cabal_name, actor_id, symbol, title, payload, status, created_at,
			updated_at)
			VALUES ($1, 'proposal', 'proposals', $2, $3, 'Alpha', $4, 'AAPLx', $5, $6, 'open', now(), now())`,
			h.IDs.NewV7(), resultProposal(i), chaosCabal(2), chaosCreator(i),
			feed.RenderTitle(feed.KindProposal, payload), payload.JSON())
		if err != nil {
			t.Fatalf("seed proposal %d: %v", i, err)
		}
	}
}

func resultCloseEvent(_ *rand.Rand, i int) events.Event {
	if i >= resultProposals {
		return events.CabalCreated{V: 1, CabalID: chaosCabal(i), CreatorID: chaosCreator(i), Name: "Alpha"}
	}
	proposal, cabal := resultProposal(i), chaosCabal(2)
	switch i {
	case 0:
		return events.ProposalPassed{V: 1, ProposalID: proposal, CabalID: cabal}
	case 1:
		return events.ProposalFailed{V: 1, ProposalID: proposal, CabalID: cabal}
	default:
		return events.ProposalExpired{V: 1, ProposalID: proposal, CabalID: cabal}
	}
}

func TestFeedResult_Redelivery_NoDuplicateAndNoPartialRow(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedResultProposals(t, h)
		return chaosFeedConsumer(t, h)
	}, resultCloseEvent)
}

const fillProposals = 3

func fillProposal(i int) uuid.UUID {
	return uuid.NewSHA1(uuid.Nil, []byte("fill-proposal"+strconv.Itoa(i)))
}

func seedFillProposals(t *testing.T, h testkit.Harness) {
	t.Helper()
	for i := range fillProposals {
		payload := feed.Payload{
			CabalName: "Alpha", ActorName: "Old", Symbol: "AAPLx", AssetName: "Apple", Action: feed.ActionBuy,
			Status: "passed", AssetKind: feed.AssetKindEquity, Outcome: feed.OutcomePassed, ProposalID: fillProposal(i),
		}
		for _, kind := range []string{"proposal", "result"} {
			_, err := h.Pool.Exec(t.Context(), `INSERT INTO feed_objects
				(id, kind, ref_type, ref_id, cabal_id, cabal_name, actor_id, symbol, title, payload, status, created_at,
				updated_at)
				VALUES ($1, $2, 'proposals', $3, $4, 'Alpha', $5, 'AAPLx', 't', $6, 'passed', now(), now())`,
				h.IDs.NewV7(), kind, fillProposal(i), chaosCabal(2), chaosCreator(i), payload.JSON())
			if err != nil {
				t.Fatalf("seed %s %d: %v", kind, i, err)
			}
		}
	}
}

func fillEvent(_ *rand.Rand, i int) events.Event {
	source := func(p int) events.TradeSource { return events.TradeSource{Kind: "proposal", ID: fillProposal(p)} }
	swap := func(n int) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte("fill-swap"+strconv.Itoa(n))) }
	stock := marketfake.AAPLx().Mint.Address()
	confirmed := func(p, n, batch int) events.Event {
		return events.TradeConfirmed{
			V: 1, SwapID: swap(n), CabalID: chaosCabal(2), Source: source(p), SourceBatchSize: batch, Action: "buy",
			Symbol: "AAPLx", InMint: usdcMint, InAmount: 100, OutMint: stock, OutAmount: 50,
			USDCMicros: money.MicrosFromUint64(100), ConfirmedAt: time.Date(2026, 10, 3, 8, 41, i, 0, time.UTC),
		}
	}
	switch i {
	case 0:
		return confirmed(0, 0, 2)
	case 1:
		return confirmed(0, 1, 2)
	case 2:
		return confirmed(1, 2, 2)
	case 3:
		return events.TradeFailed{
			V: 1, SwapID: swap(3), CabalID: chaosCabal(2), Source: source(1), SourceBatchSize: 2,
			Action: "buy", FailureCode: "price_moved",
		}
	case 4:
		return events.TradeFailed{
			V: 1, SwapID: swap(4), CabalID: chaosCabal(2), Source: source(2), SourceBatchSize: 1,
			Action: "buy", FailureCode: "no_route",
		}
	default:
		return events.CabalCreated{V: 1, CabalID: chaosCabal(i), CreatorID: chaosCreator(i), Name: "Alpha"}
	}
}

func TestFeedResult_Fill_RollbackAndReplayApplyEachSwapOnce(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedFillProposals(t, h)
		cards := make([]identity.UserCard, chaosCabals)
		for p := range chaosCabals {
			cards[p] = identity.UserCard{ID: ids.UserIDFrom(chaosCreator(p)), Handle: "p" + strconv.Itoa(p)}
		}
		deps := module.Deps{Pool: h.Pool, UoW: db.New(h.Pool, h.IDs, h.Clock), IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}
		for _, c := range social.New(deps, social.WithUsers(fakes.NewIdentity(cards, nil)),
			social.WithCabals(chaosNames()), social.WithTallies(chaosTallies()),
			social.WithAssets(marketfake.NewCatalog(marketfake.AAPLx()))).Consumers() {
			if c.Durable == "social_feed" {
				return c
			}
		}
		t.Fatal("social_feed is not registered")
		return bus.Consumer{}
	}, fillEvent)
}
