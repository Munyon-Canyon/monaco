package adapters

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
)

func TestWireAdminProposal_aRuleProposalHasNoTradeFields(t *testing.T) {
	t.Parallel()
	view := app.ProposalView{
		Kind: domain.KindRule, Rule: &app.RuleView{Change: domain.ShowSidesChange(false, true)},
		Status: domain.StatusOpen,
	}
	got := wireAdminProposal(app.AdminProposal{Detail: app.ProposalDetail{Proposal: view}})
	if got.Kind != "rule" || got.Symbol != "" || got.QuoteOutAmount != 0 || got.UsdcMicros != nil {
		t.Fatalf("wireAdminProposal = %+v, want a rule kind with empty trade fields", got)
	}
}

func TestOrEmptyAndOrZero(t *testing.T) {
	t.Parallel()
	text, number := "kai", int64(5)
	if orEmpty(&text) != "kai" || orEmpty(nil) != "" || orZero(&number) != 5 || orZero(nil) != 0 {
		t.Fatal("orEmpty and orZero must dereference a value and return zero for nil")
	}
}
