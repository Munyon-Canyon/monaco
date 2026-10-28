package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestProposalHead_readsTheKindSymbolAndThesisAndRefusesAMissingProposal(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	gen := ids.Real{}
	withThesis, without := gen.NewV7(), gen.NewV7()
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	for id, thesis := range map[any]any{withThesis: "Small add before earnings.", without: nil} {
		exec(t, db, `INSERT INTO proposals
			(id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, quote_out_amount, threshold, status,
			 expires_at, created_at, updated_at, thesis)
			VALUES ($1, $2, $3, 'sell', 'TSLAx', 'mint', 5000000, 1, 'majority', 'open', $4, $5, $5, $6)`,
			id, gen.NewV7(), gen.NewV7(), now.Add(time.Hour), now, thesis)
	}
	heads := adapters.NewProposalHeads(db)

	got, err := heads.ProposalHead(t.Context(), ids.ProposalIDFrom(withThesis))
	bare, bareErr := heads.ProposalHead(t.Context(), ids.ProposalIDFrom(without))
	_, missing := heads.ProposalHead(t.Context(), ids.ProposalIDFrom(gen.NewV7()))
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	_, down := heads.ProposalHead(canceled, ids.ProposalIDFrom(withThesis))

	if want := (port.ProposalHead{Kind: "sell", Symbol: "TSLAx", Thesis: "Small add before earnings."}); err != nil ||
		got != want {
		t.Errorf("ProposalHead = %+v, %v, want %+v", got, err, want)
	}
	if bareErr != nil || bare.Thesis != "" {
		t.Errorf("a proposal with no thesis = %+v, %v, want an empty thesis", bare, bareErr)
	}
	if errs.CodeOf(missing) != errs.CodeProposalNotFound {
		t.Errorf("a missing proposal = %v, want %s", missing, errs.CodeProposalNotFound)
	}
	if errs.CodeOf(down) != errs.CodeDBUnavailable {
		t.Errorf("a canceled read = %v, want %s", down, errs.CodeDBUnavailable)
	}
}
