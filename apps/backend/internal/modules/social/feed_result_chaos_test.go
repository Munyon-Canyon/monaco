//go:build faultpoints

package social_test

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
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
