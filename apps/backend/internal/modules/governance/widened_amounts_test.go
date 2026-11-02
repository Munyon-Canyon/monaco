package governance_test

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestWidenedAmounts_aProposalAtTheInt64LimitReadsBackAndOnePastItFailsLoudly(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withGovernanceAndAdmin())
	c := seedHandledCabal(t, s)
	p := seedLookupProposal(t, s, c, "open")
	q := sqlc.New(s.DB())
	read := func() (sqlc.GetProposalRow, error) {
		return q.GetProposal(t.Context(), sqlc.GetProposalParams{CallerID: p.voter.UUID(), ID: p.id})
	}

	if _, err := s.DB().
		Exec(t.Context(), `UPDATE proposals SET quote_out_amount = $2 WHERE id = $1`, p.id, int64(math.MaxInt64)); err != nil {
		t.Fatal(err)
	}
	got, err := read()
	if err != nil || got.QuoteOutAmount.Int64 != math.MaxInt64 {
		t.Fatalf("GetProposal = %+v, %v, want quote_out_amount at int64 max", got.QuoteOutAmount, err)
	}

	if _, err := s.DB().Exec(t.Context(),
		`UPDATE proposals SET quote_out_amount = 25000000000000000000::numeric WHERE id = $1`, p.id); err != nil {
		t.Fatalf("a 25-token amount at 18 decimals must fit the column: %v", err)
	}
	if got, err := read(); err == nil {
		t.Fatalf("GetProposal = %+v, want an error: an amount past int64 must not narrow silently", got.QuoteOutAmount)
	}
}
