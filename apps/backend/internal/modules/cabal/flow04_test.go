package cabal_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func leaving(t *testing.T, extra ...scenario.Option) *scenario.Scenario {
	t.Helper()
	return cabalScenarioWith(t, nil, extra...)
}

func TestFlow04_LeaveCabal_OK(t *testing.T) {
	t.Parallel()
	flows.F04LeaveCabalOK(leaving(t, scenario.WithPostHog(t)))
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
	flows.F04LeaveCabalLeaveHoldsShares(leaving(t))
}

func TestFlow04_LeaveCabal_LeaveLastMemberPotNotEmpty(t *testing.T) {
	t.Parallel()
	flows.F04LeaveCabalLeaveLastMemberPotNotEmpty(leaving(t))
}

func TestFlow04_LeaveCabal_LeaveCreatorWithMembers(t *testing.T) {
	t.Parallel()
	flows.F04LeaveCabalLeaveCreatorWithMembers(leaving(t))
}

func TestFlow04_LeaveCabal_PriceUnavailable(t *testing.T) {
	t.Parallel()
	flows.F04LeaveCabalPriceUnavailable(leaving(t))
}
