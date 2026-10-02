//go:build faultpoints

package governance_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow10_CastVote_CrashAfterPublish(t *testing.T) {
	t.Parallel()
	flows.F10CastVoteCrashAfterPublish(scenario.New(t, withGovernance()))
}
