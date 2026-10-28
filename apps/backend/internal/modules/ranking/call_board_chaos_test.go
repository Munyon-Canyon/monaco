//go:build faultpoints

package ranking_test

import (
	"math/rand/v2"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestCallBoard_convergesUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		return rankingConsumer(t, h, "ranking_call_board")
	}, func(_ *rand.Rand, i int) events.Event {
		switch i % 5 {
		case 0:
			return events.CallResolved{V: 1}
		case 1:
			return events.VoteCast{V: 1}
		case 2:
			return events.ProposalCreated{V: 1}
		case 3:
			return events.UserProfileUpdated{V: 1, Fields: []string{"handle"}}
		}
		return events.CabalUpdated{V: 1}
	})
}
