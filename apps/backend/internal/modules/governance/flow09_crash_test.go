//go:build faultpoints

package governance_test

import (
	"net/http"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow09_ProposeTrade_CrashAfterPublish(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	w.scenario(t).Given(w.asMember(), scenario.HoldRelay()).
		When(
			scenario.Post(w.proposals(), buyAAPL),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.PublishCrashingAt(faultpoint.AfterPublish),
		).
		Then(
			scenario.ExpectEvents(events.TypeProposalCreated, 1),
			scenario.EventuallyPublished(events.TypeProposalCreated, 1),
		)
}
