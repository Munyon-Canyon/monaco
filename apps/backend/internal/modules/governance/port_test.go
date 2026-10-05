package governance_test

import (
	"context"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestPort_statusFollowsTheGuardedTransition(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	port := governance.New(module.Deps{Pool: d.pool}).Queries()
	p := d.buy(d.ids.NewV7())
	d.insert(t, p)
	id := ids.ProposalIDFrom(p.ID)
	if got, err := port.Status(t.Context(), id); err != nil || got != domain.StatusOpen {
		t.Fatalf("Status = %s, %v, want open", got, err)
	}
	d.transition(t, p.ID, "open", "passed", "")
	d.transition(t, p.ID, "passed", "voided", "Duplicate.")
	if got, err := port.Status(t.Context(), id); err != nil || got != domain.StatusVoided {
		t.Fatalf("Status after the void = %s, %v, want voided", got, err)
	}
}

func TestPort_statusErrors(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	port := governance.New(module.Deps{Pool: d.pool}).Queries()
	unknown := ids.ProposalIDFrom(d.ids.NewV7())
	if _, err := port.Status(t.Context(), unknown); errs.CodeOf(err) != errs.CodeProposalNotFound {
		t.Errorf("Status of an unknown proposal err = %v, want proposal_not_found", err)
	}
	p := d.buy(d.ids.NewV7())
	d.insert(t, p)
	d.transition(t, p.ID, "open", "gremlins", "")
	if _, err := port.Status(t.Context(), ids.ProposalIDFrom(p.ID)); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Errorf("Status of an unknown stored status err = %v, want decode_failed", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := port.Status(ctx, ids.ProposalIDFrom(p.ID)); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("Status on a cancelled context err = %v, want internal", err)
	}
}

func TestPort_proposedMintsNameOpenAndPassedProposalsOnly(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	mints := governance.New(module.Deps{Pool: d.pool})
	for i, status := range []string{"open", "passed", "voided", "open"} {
		p := d.buy(d.ids.NewV7())
		p.Mint = []string{"MintOpen", "MintPassed", "MintVoided", "MintOpen"}[i]
		d.insert(t, p)
		if status != "open" {
			d.transition(t, p.ID, "open", status, "")
		}
	}
	got, err := mints.ProposedMints(t.Context())
	if err != nil || !slices.Equal(got, []chain.SolanaAddress{"MintOpen", "MintPassed"}) {
		t.Fatalf("ProposedMints = %v, %v, want MintOpen and MintPassed once each", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := mints.ProposedMints(ctx); err == nil {
		t.Fatal("ProposedMints on a cancelled context succeeded")
	}
}
