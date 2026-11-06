//go:build faultpoints

package social_test

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func chatMomentEvent(_ *rand.Rand, i int) events.Event {
	cabal := chaosCabal(1)
	at := time.Date(2026, 9, 30, 14, 0, 0, 0, time.UTC)
	shares := money.UnitsFromUint64(2)
	switch i {
	case 0:
		return events.CabalCreated{V: 1, CabalID: cabal, CreatorID: chaosCreator(1), Name: "Alpha"}
	case 1:
		return events.CabalMemberJoined{V: 1, CabalID: cabal, UserID: chaosCreator(2), RequestID: chaosProposal()}
	case 2:
		return events.CabalMemberJoined{V: 1, CabalID: cabal, UserID: chaosCreator(2), RequestID: resultProposal(0)}
	case 6:
		return events.CabalMemberJoined{V: 1, CabalID: cabal, UserID: chaosCreator(3)}
	case 7:
		return events.ProposalFailed{V: 1, ProposalID: resultProposal(0), CabalID: cabal, Kind: "buy"}
	case 8:
		return events.ProposalExpired{V: 1, ProposalID: resultProposal(2), CabalID: cabal, Kind: "buy"}
	case 9:
		return events.ProposalFailed{V: 1, ProposalID: resultProposal(0), CabalID: cabal, Kind: "rule"}
	case 3:
		return events.TradeConfirmed{
			V: 1, SwapID: uuid.NewSHA1(uuid.Nil, []byte("fill-buy")), CabalID: cabal, Action: "buy",
			Source:  events.TradeSource{Kind: "proposal", ID: resultProposal(1)},
			OutMint: marketfake.AAPLx().Mint.Address(), OutAmount: shares,
			USDCMicros: money.MicrosFromUint64(393_500_000), ConfirmedAt: at,
		}
	case 4:
		return events.TradeConfirmed{
			V: 1, SwapID: uuid.NewSHA1(uuid.Nil, []byte("fill-sell")), CabalID: cabal, Action: "sell",
			InMint: marketfake.AAPLx().Mint.Address(), InAmount: shares,
			USDCMicros: money.MicrosFromUint64(400_000_000), ConfirmedAt: at.Add(time.Hour),
		}
	default:
		return events.TradeConfirmed{
			V: 1, SwapID: uuid.NewSHA1(uuid.Nil, []byte("fill-other")), CabalID: cabal, Action: "swap",
			ConfirmedAt: at.Add(2 * time.Hour),
		}
	}
}

func seedMomentProposals(t *testing.T, h testkit.Harness) {
	t.Helper()
	for i := range resultProposals {
		payload := feed.Payload{
			CabalName: "Alpha", ActorName: "Old", Symbol: "AAPLx", AssetName: "Apple", Action: feed.ActionBuy,
			USDCMicros: money.MicrosFromUint64(300_000_000), Status: "open", AssetKind: feed.AssetKindEquity,
		}
		_, err := h.Pool.Exec(t.Context(), `INSERT INTO feed_objects
			(id, kind, ref_type, ref_id, cabal_id, cabal_name, actor_id, symbol, title, payload, status, created_at,
			updated_at)
			VALUES ($1, 'proposal', 'proposals', $2, $3, 'Alpha', $4, 'AAPLx', 't', $5, 'open', now(), now())`,
			h.IDs.NewV7(), resultProposal(i), chaosCabal(1), chaosCreator(1), payload.JSON())
		if err != nil {
			t.Fatalf("seed proposal %d: %v", i, err)
		}
	}
}

func TestChatMomentsConsumer_Chaos_RedeliveryWritesEachMomentOnce(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		_, err := h.Pool.Exec(t.Context(), `INSERT INTO users
			(id, privy_user_id, auth_state, login_provider, auth_state_changed_at, created_at, updated_at)
			VALUES ($1, 'did:privy:chat-moments', 'CREATED', 'sms', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')`, chaosCreator(1))
		if err != nil {
			t.Fatalf("seed creator: %v", err)
		}
		_, err = h.Pool.Exec(t.Context(), `INSERT INTO cabals
			(id, name, creator_id, join_mode, voter_mode, threshold, proposal_expiry_seconds, created_at, updated_at)
			VALUES ($1, 'Alpha', $2, 'open', 'all', 'majority', 86400, $3, $3)`,
			chaosCabal(1), chaosCreator(1), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatalf("seed cabal: %v", err)
		}
		seedMomentProposals(t, h)
		deps := module.Deps{Pool: h.Pool, UoW: db.New(h.Pool, h.IDs, h.Clock), IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}
		mod := social.New(deps, social.WithAssets(marketfake.NewCatalog(marketfake.AAPLx())),
			social.WithTallies(chaosTallies()))
		for _, c := range mod.Consumers() {
			if c.Durable == "social_chat_moments" {
				return c
			}
		}
		t.Fatal("social_chat_moments is not registered")
		return bus.Consumer{}
	}, chatMomentEvent)
}
