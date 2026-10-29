package governance_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow38_ProposeRuleChange_OK(t *testing.T) {
	t.Parallel()
	flows.F38ProposeRuleChangeOK(scenario.New(t, withGovernance(), withFakes()))
}

func TestFlow38_ProposeRuleChange_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F38ProposeRuleChangeInvalidInput(scenario.New(t, withGovernance()))
}

func TestFlow38_ProposeRuleChange_NotAVoter(t *testing.T) {
	t.Parallel()
	flows.F38ProposeRuleChangeNotAVoter(scenario.New(t, withGovernance()))
}

func TestFlow38_ProposeRuleChange_NotCabalMember(t *testing.T) {
	t.Parallel()
	flows.F38ProposeRuleChangeNotCabalMember(scenario.New(t, withGovernance()))
}

func TestFlow38_ProposeRuleChange_RuleUnchanged(t *testing.T) {
	t.Parallel()
	flows.F38ProposeRuleChangeRuleUnchanged(scenario.New(t, withGovernance()))
}

func TestFlow38_ProposeRuleChange_RuleChangeOpen(t *testing.T) {
	t.Parallel()
	flows.F38ProposeRuleChangeRuleChangeOpen(scenario.New(t, withGovernance()))
}
