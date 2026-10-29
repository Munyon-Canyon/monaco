//go:build faultpoints

package notify_test

import (
	"context"
	"math/rand/v2"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type chaosVotes struct{ cabals *sync.Map }

func (v chaosVotes) cabalOf(id ids.ProposalID) int {
	n, _ := v.cabals.Load(id.UUID())
	cabal, _ := n.(int)
	return cabal
}

func (v chaosVotes) VoteState(_ context.Context, id ids.ProposalID) (governanceport.VoteState, error) {
	return decidingState(chaosCabal(v.cabalOf(id))), nil
}

func (chaosVotes) DueForReminder(
	context.Context, time.Time, governanceport.ReminderCursor, int,
) ([]governanceport.DueReminder, error) {
	return nil, nil
}

func (v chaosVotes) NonVoters(_ context.Context, id ids.ProposalID) ([]ids.UserID, error) {
	if v.cabalOf(id) == 0 {
		return []ids.UserID{
			ids.UserIDFrom(chaosRecipient(0)), ids.UserIDFrom(chaosRecipient(1)), ids.UserIDFrom(chaosRecipient(2)),
		}, nil
	}
	return []ids.UserID{ids.UserIDFrom(chaosRecipient(1)), ids.UserIDFrom(chaosRecipient(3))}, nil
}

func chaosVoteCast(i int, cabals *sync.Map) events.Event {
	proposal := uuid.NewSHA1(uuid.NameSpaceOID, []byte("notify-chaos-vote-proposal-"+strconv.Itoa(i)))
	cabals.Store(proposal, i%2)
	return events.VoteCast{
		V: 1, ProposalID: proposal, CabalID: chaosCabal(i % 2), VoterID: chaosRecipient((i + 2) % chaosUsers),
		Choice: "yes", CastAt: time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC),
	}
}

func TestNotify_VoteDecidingKind_ConvergesUnderChaos(t *testing.T) {
	t.Parallel()
	cabals := &sync.Map{}
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedChaosUsers(t, h)
		deps := module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, UoW: db.New(h.Pool, h.IDs, h.Clock)}
		m := notify.New(deps, notify.WithSender(&testkit.FakeSender{}), notify.WithCabals(chaosCabals()),
			notify.WithVoteStates(chaosVotes{cabals: cabals}))
		return m.Consumers()[0]
	}, func(_ *rand.Rand, i int) events.Event { return chaosVoteCast(i, cabals) })
}
