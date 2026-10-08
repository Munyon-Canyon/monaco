package governance_test

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
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

func TestPort_retryableIsTrueForPassedOrSwapFailedOnly(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	port := governance.New(module.Deps{Pool: d.pool}).Queries()
	unknown := ids.ProposalIDFrom(d.ids.NewV7())
	if _, err := port.Retryable(t.Context(), unknown); errs.CodeOf(err) != errs.CodeProposalNotFound {
		t.Errorf("Retryable of an unknown proposal err = %v, want proposal_not_found", err)
	}
	p := d.buy(d.ids.NewV7())
	d.insert(t, p)
	id := ids.ProposalIDFrom(p.ID)
	steps := []struct {
		status, reason string
		want           bool
	}{
		{"open", "", false},
		{"passed", "", true},
		{"execution_blocked", "no_route", false},
		{"execution_blocked", "swap_failed", true},
	}
	for _, step := range steps {
		if _, err := d.pool.Exec(t.Context(),
			`UPDATE proposals SET status = $2, status_reason = nullif($3, '') WHERE id = $1`,
			p.ID, step.status, step.reason); err != nil {
			t.Fatal(err)
		}
		if got, err := port.Retryable(t.Context(), id); err != nil || got != step.want {
			t.Errorf("Retryable of %s %q = %t, %v, want %t", step.status, step.reason, got, err, step.want)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := port.Retryable(ctx, id); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("Retryable on a cancelled context err = %v, want internal", err)
	}
}

func TestPort_proposerNamesWhoOpenedTheProposal(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	port := governance.New(module.Deps{Pool: d.pool}).Queries()
	p := d.buy(d.ids.NewV7())
	d.insert(t, p)
	d.transition(t, p.ID, "open", "passed", "")
	got, err := port.Proposer(t.Context(), ids.ProposalIDFrom(p.ID))
	if err != nil || got != ids.UserIDFrom(p.ProposerID) {
		t.Fatalf("Proposer = %s, %v, want %s", got, err, p.ProposerID)
	}
}

func TestPort_proposerErrors(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	port := governance.New(module.Deps{Pool: d.pool}).Queries()
	unknown := ids.ProposalIDFrom(d.ids.NewV7())
	if _, err := port.Proposer(t.Context(), unknown); errs.CodeOf(err) != errs.CodeProposalNotFound {
		t.Errorf("Proposer of an unknown proposal err = %v, want proposal_not_found", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := port.Proposer(ctx, unknown); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Errorf("Proposer on a cancelled context err = %v, want db_unavailable so a consumer naks", err)
	}
}

func (d proposalDB) ballotAfter(t *testing.T, proposal, voter uuid.UUID, choice string, after time.Duration) {
	t.Helper()
	err := d.q.UpsertBallot(t.Context(), sqlc.UpsertBallotParams{
		ProposalID: proposal, VoterID: voter, Choice: choice, CastAt: d.now.Add(after),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPort_votersNameEveryBallotInCastOrderAndNobodyElse(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	voters := governance.New(module.Deps{Pool: d.pool}).Voters()
	eligible := []uuid.UUID{d.ids.NewV7(), d.ids.NewV7(), d.ids.NewV7(), d.ids.NewV7()}
	slices.SortFunc(eligible, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	low, mid, high, idle := eligible[0], eligible[1], eligible[2], eligible[3]
	p, other := d.buy(eligible...), d.buy(idle)
	d.insert(t, p)
	d.insert(t, other)
	d.ballotAfter(t, p.ID, mid, "yes", time.Minute)
	d.ballotAfter(t, p.ID, low, "no", time.Minute)
	d.ballotAfter(t, p.ID, high, "yes", 0)
	d.ballotAfter(t, other.ID, idle, "yes", 0)

	got, err := voters.Voters(t.Context(), ids.ProposalIDFrom(p.ID))

	want := []ids.UserID{ids.UserIDFrom(high), ids.UserIDFrom(low), ids.UserIDFrom(mid)}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("Voters = %v, %v, want %v: both choices, earliest cast first, ties by voter, and no one who "+
			"is eligible but did not vote or voted on another proposal", got, err, want)
	}
}

func TestPort_votersErrors(t *testing.T) {
	t.Parallel()
	d := newProposalDB(t)
	voters := governance.New(module.Deps{Pool: d.pool}).Voters()
	p := d.buy(d.ids.NewV7())
	d.insert(t, p)
	unknown := d.ids.NewV7()
	for name, id := range map[string]uuid.UUID{"a proposal nobody voted on": p.ID, "an unknown proposal": unknown} {
		if got, err := voters.Voters(t.Context(), ids.ProposalIDFrom(id)); err != nil || len(got) != 0 {
			t.Errorf("Voters of %s = %v, %v, want none and no error", name, got, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := voters.Voters(ctx, ids.ProposalIDFrom(p.ID)); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Errorf("Voters on a cancelled context err = %v, want db_unavailable so a consumer naks", err)
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
