package governance_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type proposalHints struct{ updated []string }

func (*proposalHints) ProposalCreated(context.Context, ids.CabalID, ids.ProposalID) {}

func (h *proposalHints) ProposalUpdated(_ context.Context, cabal ids.CabalID, proposal ids.ProposalID) {
	h.updated = append(h.updated, cabal.String()+":"+proposal.String())
}

func TestProposalHints_PublishesKeysAndPayloads(t *testing.T) {
	t.Parallel()
	got := &publishedHints{}
	g := testkit.NewIDs(testkit.RandSeed(t))
	cabal, proposal := ids.CabalIDFrom(g.NewV7()), ids.ProposalIDFrom(g.NewV7())
	h := adapters.Hints{Publish: got}
	h.ProposalCreated(t.Context(), cabal, proposal)
	h.ProposalUpdated(t.Context(), cabal, proposal)
	app.NoHints{}.ProposalCreated(t.Context(), cabal, proposal)
	app.NoHints{}.ProposalUpdated(t.Context(), cabal, proposal)
	want := []string{
		"cabal." + cabal.String() + ".proposal_created:{\"proposal_id\":\"" + proposal.String() + "\"}",
		"cabal." + cabal.String() + ".proposal_updated:{\"proposal_id\":\"" + proposal.String() + "\"}",
	}
	if len(got.calls) != 2 || got.calls[0] != want[0] || got.calls[1] != want[1] {
		t.Fatalf("hints = %q, want %q", got.calls, want)
	}
}

type publishedHints struct{ calls []string }

func (h *publishedHints) PublishHint(_ context.Context, key string, payload []byte) {
	h.calls = append(h.calls, key+":"+string(payload))
}

func TestProposalHints_CastVoteAndWithdrawPublishAfterCommit(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	hints := &proposalHints{}
	voteProposal, voters := d.open(t, 1)
	vote := app.NewCastVoteHandler(d.uow, d.pool, d.clk, threshold{rule: domain.RuleMajority}, hints)
	d.mustCast(t, vote, voteProposal.ID, voters[0], domain.ChoiceYes)
	withdrawProposal, proposer := d.buy(d.ids.NewV7()), d.ids.NewV7()
	withdrawProposal.ProposerID = proposer
	d.insert(t, withdrawProposal)
	withdraw := app.NewWithdrawProposalHandler(d.uow, d.clk, hints)
	if err := withdraw.Handle(actorContext(t, proposer), app.WithdrawProposal{
		ProposalID: ids.ProposalIDFrom(withdrawProposal.ID), ActorID: ids.UserIDFrom(proposer),
	}); err != nil {
		t.Fatalf("WithdrawProposal() = %v", err)
	}
	if len(hints.updated) != 2 || len(d.payloads(t, voteProposal.ID, events.TypeProposalPassed)) != 1 ||
		len(d.payloads(t, withdrawProposal.ID, events.TypeProposalWithdrawn)) != 1 {
		t.Fatalf("hints = %q, committed vote/withdraw events missing", hints.updated)
	}
}

func TestProposalHints_RefusalsPublishNothing(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	hints := &proposalHints{}
	p, voters := d.open(t, 1)
	vote := app.NewCastVoteHandler(d.uow, d.pool, d.clk, threshold{rule: domain.RuleMajority}, hints)
	if _, err := d.cast(t.Context(), vote, p.ID, d.ids.NewV7(), domain.ChoiceYes); err == nil {
		t.Fatal("non-voter ballot = nil")
	}
	d.mustCast(t, vote, p.ID, voters[0], domain.ChoiceYes)
	if _, err := d.cast(t.Context(), vote, p.ID, voters[0], domain.ChoiceYes); err == nil {
		t.Fatal("closed ballot = nil")
	}
	withdraw := app.NewWithdrawProposalHandler(d.uow, d.clk, hints)
	open := d.buy(d.ids.NewV7())
	d.insert(t, open)
	if err := withdraw.Handle(actorContext(t, d.ids.NewV7()), app.WithdrawProposal{
		ProposalID: ids.ProposalIDFrom(open.ID), ActorID: ids.UserIDFrom(d.ids.NewV7()),
	}); err == nil {
		t.Fatal("non-proposer withdrawal = nil")
	}
	if err := withdraw.Handle(actorContext(t, voters[0]), app.WithdrawProposal{
		ProposalID: ids.ProposalIDFrom(p.ID), ActorID: ids.UserIDFrom(voters[0]),
	}); err == nil {
		t.Fatal("closed withdrawal = nil")
	}
	if len(hints.updated) != 1 {
		t.Fatalf("hints = %q, want only accepted ballot", hints.updated)
	}
}

func TestProposalHints_PublishFailure_CommandSucceeds(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	drops := &proposalHints{}
	p, voters := d.open(t, 1)
	vote := app.NewCastVoteHandler(d.uow, d.pool, d.clk, threshold{rule: domain.RuleMajority}, drops)
	d.mustCast(t, vote, p.ID, voters[0], domain.ChoiceYes)
	proposer := d.ids.NewV7()
	withdrawn := d.buy(proposer)
	withdrawn.ProposerID = proposer
	d.insert(t, withdrawn)
	withdraw := app.NewWithdrawProposalHandler(d.uow, d.clk, drops)
	if err := withdraw.Handle(actorContext(t, proposer), app.WithdrawProposal{
		ProposalID: ids.ProposalIDFrom(withdrawn.ID), ActorID: ids.UserIDFrom(proposer),
	}); err != nil {
		t.Fatalf("WithdrawProposal() = %v", err)
	}
	if len(drops.updated) != 2 || len(d.payloads(t, p.ID, events.TypeProposalPassed)) != 1 ||
		len(d.payloads(t, withdrawn.ID, events.TypeProposalWithdrawn)) != 1 {
		t.Fatalf("dropped hint prevented vote commit: %q", drops.updated)
	}
}

func actorContext(t *testing.T, id uuid.UUID) context.Context {
	t.Helper()
	return observability.WithActor(t.Context(), "user:"+id.String())
}
