package funding_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func TestFlowStep30_EVMWithdrawal(t *testing.T) {
	t.Parallel()
	flows.F30WithdrawEVMOK(flow15Scenario(t))
}
