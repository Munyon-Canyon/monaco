//go:build faultpoints

package notify_test

import (
	"context"
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type everyCard struct{}

func (everyCard) PendingVotes(context.Context, ids.UserID) ([]governanceport.CatchUpProposal, error) {
	return nil, nil
}

func (everyCard) ProposalCards(
	_ context.Context, _ ids.UserID, proposals []ids.ProposalID,
) (map[ids.ProposalID]governanceport.CatchUpProposal, error) {
	out := make(map[ids.ProposalID]governanceport.CatchUpProposal, len(proposals))
	for _, id := range proposals {
		out[id] = governanceport.CatchUpProposal{
			ID: id, Kind: "buy", Symbol: "NVDA", Status: governanceport.StatusPassed,
			CreatedAt: time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC),
		}
	}
	return out, nil
}

func catchUpChaosEvent(i int) events.Event {
	proposal := uuid.NewSHA1(uuid.NameSpaceOID, []byte("catch-up-chaos-proposal-"+strconv.Itoa(i/3)))
	voter := chaosRecipient(i % 3)
	cabal := chaosCabal(0)
	base := time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC)
	switch i % 3 {
	case 0:
		return events.VoteCast{
			V: 1, ProposalID: proposal, CabalID: cabal, VoterID: voter, Choice: "yes", CastAt: base,
		}
	case 1:
		return events.VoteRetracted{
			V: 1, ProposalID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("catch-up-chaos-proposal-"+strconv.Itoa(i/3))),
			CabalID: cabal, VoterID: chaosRecipient((i - 1) % 3), Choice: "yes", RetractedAt: base.Add(time.Minute),
		}
	}
	return events.ProposalPassed{V: 1, ProposalID: proposal, CabalID: cabal}
}

func TestCatchUp_AVoteAndItsRetractionConvergeInAnyOrderUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedChaosUsers(t, h)
		reads := app.NewReadCatchUp(app.CatchUpPorts{
			Governance: everyCard{}, Replies: &catchUpReplies{counts: map[ids.ProposalID]int{}},
			Reads: h.Pool, Clock: h.Clock,
		})
		clears := app.NewCatchUpClears(reads, chaosCabals(), &hintRecorder{})
		return bus.Consumer{Durable: adapters.CatchUpDurable, Handlers: adapters.CatchUpHandlers(clears)}
	}, func(_ *rand.Rand, i int) events.Event { return catchUpChaosEvent(i) })
}
