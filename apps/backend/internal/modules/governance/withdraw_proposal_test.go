package governance_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (d voteDB) withdraw(t *testing.T, proposal, actor uuid.UUID) error {
	t.Helper()
	ctx := observability.WithActor(t.Context(), "user:"+actor.String())
	return app.NewWithdrawProposalHandler(d.uow, d.clk).Handle(ctx, app.WithdrawProposal{
		ProposalID: ids.ProposalIDFrom(proposal), ActorID: ids.UserIDFrom(actor),
	})
}

func TestWithdrawProposal_theProposersOwnBallotDoesNotBlockIt(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	proposer, other := d.ids.NewV7(), d.ids.NewV7()
	p := d.buy(proposer, other, d.ids.NewV7())
	p.ProposerID = proposer
	d.insert(t, p)
	d.mustCast(t, d.handler(threshold{rule: domain.RuleMajority}), p.ID, proposer, domain.ChoiceYes)
	if err := d.withdraw(t, p.ID, proposer); err != nil {
		t.Fatalf("withdraw after only the proposer's ballot = %v, want nil", err)
	}
	if r := d.row(t, p.ID); r.status.String != "withdrawn" || !r.updatedAt.Equal(d.clk.Now()) {
		t.Errorf("row = %+v, want withdrawn at %s", r, d.clk.Now())
	}
	got := d.payloads(t, p.ID, events.TypeProposalWithdrawn)
	if len(got) != 1 || d.terminal(t, p.ID) != 0 {
		t.Fatalf("proposal.withdrawn payloads = %s, want exactly one and no tally event", got)
	}
	var ev events.ProposalWithdrawn
	if err := json.Unmarshal(got[0], &ev); err != nil {
		t.Fatal(err)
	}
	if want := (events.ProposalWithdrawn{V: 1, ProposalID: p.ID, CabalID: p.CabalID, ProposerID: proposer}); ev != want {
		t.Fatalf("proposal.withdrawn = %+v, want %+v", ev, want)
	}
	if err := d.withdraw(t, p.ID, other); errs.CodeOf(err) != errs.CodeNotProposer {
		t.Fatalf("a non-proposer withdrawing a closed proposal = %v, want not_proposer", err)
	}
}

func TestWithdrawProposal_aFailedUpdateIsInternalAndAppendsNothing(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	proposer := d.ids.NewV7()
	p := d.buy(proposer)
	p.ProposerID = proposer
	d.insert(t, p)
	if _, err := d.pool.Exec(t.Context(),
		`ALTER TABLE proposals ADD CONSTRAINT no_withdraw CHECK (status <> 'withdrawn') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if err := d.withdraw(t, p.ID, proposer); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("withdraw whose update fails = %v, want internal", err)
	}
	if got := d.payloads(t, p.ID, events.TypeProposalWithdrawn); len(got) != 0 {
		t.Fatalf("proposal.withdrawn payloads = %s, want none", got)
	}
}

func TestHTTP_DeleteProposal(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.proposedBy(t, d.caller)
	uow := db.New(d.pool, d.ids, testkit.NewClock(d.now))
	h := adapters.HTTP{Withdraw: app.NewWithdrawProposalHandler(uow, testkit.NewClock(d.now)), Reads: d.reads()}
	req := api.DeleteProposalRequestObject{Id: p}
	if _, err := h.DeleteProposal(t.Context(), req); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("DeleteProposal with no actor = %v, want unauthorized", err)
	}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
	h.Reads = app.NewProposalReads(d.pool, threshold{err: errs.New(errs.CodeCabalNotFound, "t")}, d.swaps)
	if _, err := h.DeleteProposal(ctx, req); errs.CodeOf(err) != errs.CodeCabalNotFound {
		t.Fatalf("DeleteProposal whose detail read fails = %v, want cabal_not_found", err)
	}
	if r := d.row(t, p); r.status.String != "withdrawn" {
		t.Fatalf("proposal after a failed detail read is %s, want withdrawn", r.status.String)
	}
}
