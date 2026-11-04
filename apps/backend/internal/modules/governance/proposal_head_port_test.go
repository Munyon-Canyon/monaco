package governance_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_proposalHeadReadsTheThesisAndRefusesAnUnknownProposal(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	m := governance.New(module.Deps{Pool: d.pool})
	p := d.buy(d.ids.NewV7())
	p.Thesis = pgtype.Text{String: "small add before the print", Valid: true}
	d.insert(t, p)
	head, err := m.ProposalHead(t.Context(), ids.ProposalIDFrom(p.ID))
	if err != nil || head.Thesis != "small add before the print" {
		t.Fatalf("ProposalHead = %+v, %v, want the thesis", head, err)
	}
	_, err = m.ProposalHead(t.Context(), ids.ProposalIDFrom(d.ids.NewV7()))
	if errs.CodeOf(err) != errs.CodeProposalNotFound {
		t.Fatalf("ProposalHead of an unknown id = %v, want proposal_not_found", err)
	}
}
