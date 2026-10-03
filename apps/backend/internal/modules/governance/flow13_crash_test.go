//go:build faultpoints

package governance_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow13_WithdrawProposal_CrashAfterPublish(t *testing.T) {
	t.Parallel()
	flows.F13WithdrawProposalCrashAfterPublish(scenario.New(t, withGovernance()))
}
