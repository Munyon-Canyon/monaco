package cabal_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func leaveScenario(t *testing.T, extra ...scenario.Option) (*scenario.Scenario, *fakes.Treasury) {
	t.Helper()
	tr := fakes.NewTreasury()
	return cabalScenarioWith(t, []cabal.Option{cabal.WithTreasuryReads(tr)}, extra...), tr
}

func leaving(t *testing.T) *scenario.Scenario {
	t.Helper()
	s, _ := leaveScenario(t)
	return s
}

func TestFlow04_LeaveCabal_OK(t *testing.T) {
	t.Parallel()
	flows.F04LeaveCabalOK(leaving(t))
}

func TestFlow04_LeaveCabal_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F04LeaveCabalUnauthorized(leaving(t))
}

func TestFlow04_LeaveCabal_NotCabalMember(t *testing.T) {
	t.Parallel()
	flows.F04LeaveCabalNotCabalMember(leaving(t))
}

func TestFlow04_LeaveCabal_LeaveHoldsShares(t *testing.T) {
	t.Parallel()
	flows.LeaveHoldingShares(leaveScenario(t))
}

func TestFlow04_LeaveCabal_LeaveLastMemberPotNotEmpty(t *testing.T) {
	t.Parallel()
	flows.LeaveLastOfAFullPot(leaveScenario(t))
}

func TestFlow04_LeaveCabal_LeaveCreatorWithMembers(t *testing.T) {
	t.Parallel()
	flows.F04LeaveCabalLeaveCreatorWithMembers(leaving(t))
}

func TestFlow04_LeaveCabal_PriceUnavailable(t *testing.T) {
	t.Parallel()
	flows.LeaveAnUnpricedPot(leaveScenario(t))
}
