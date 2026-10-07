package governance_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (d readsDB) get(t *testing.T, proposal, caller uuid.UUID) app.ProposalDetail {
	t.Helper()
	got, err := d.reads().Get(t.Context(), app.GetProposal{
		ID: ids.ProposalIDFrom(proposal), Caller: ids.UserIDFrom(caller),
	})
	if err != nil {
		t.Fatalf("Get(%s) = %v", proposal, err)
	}
	return got
}

func (d readsDB) proposedBy(t *testing.T, proposer uuid.UUID, voters ...uuid.UUID) uuid.UUID {
	t.Helper()
	p := d.buy(append([]uuid.UUID{proposer}, voters...)...)
	p.CabalID, p.ProposerID = d.cabal, proposer
	d.insert(t, p)
	return p.ID
}

type permissions struct{ vote, withdraw bool }

func can(got app.ProposalDetail) permissions { return permissions{got.CanVote, got.CanWithdraw} }

func TestProposals_Detail_NonMemberViewOnly(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	member := d.ids.NewV7()
	p := d.proposedBy(t, d.caller, member)
	d.ballot(t, p, member, "yes")
	got := d.get(t, p, d.ids.NewV7())
	if can(got) != (permissions{}) || got.Proposal.MyBallot != "" || got.Swap != nil ||
		got.Proposal.Tally != (app.Tally{Yes: 1, Voters: 2, Needed: 2}) || len(got.Voters) != 2 {
		t.Fatalf("non-member detail = %+v, want view-only with the full tally", got)
	}
}

func TestProposals_Detail_reportsTheFrozenThresholdNeed(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	voters := []uuid.UUID{d.caller, d.ids.NewV7(), d.ids.NewV7()}
	p := d.buy(voters...)
	p.CabalID, p.ProposerID, p.Threshold = d.cabal, d.caller, "unanimous"
	d.insert(t, p)
	if got := d.get(t, p.ID, d.caller).Proposal.Tally.Needed; got != 3 {
		t.Fatalf("detail needed = %d, want 3 under the frozen unanimous rule", got)
	}
	d.seed(t, d.now.Add(time.Minute))
	page := d.list(t, app.FilterAll, 10, "")
	if len(page.Items) != 2 || page.Items[0].Tally.Needed != 1 || page.Items[1].Tally.Needed != 3 {
		t.Fatalf("list needs = %+v, want each proposal's own frozen threshold", page.Items)
	}
}

func TestProposals_Detail_votersAndPermissions(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	other := d.ids.NewV7()
	p := d.proposedBy(t, d.caller, other)
	if got := can(d.get(t, p, d.caller)); got != (permissions{vote: true, withdraw: true}) {
		t.Errorf("proposer before any ballot = %+v, want vote and withdraw", got)
	}
	d.ballot(t, p, d.caller, "yes")
	if got := can(d.get(t, p, d.caller)); got != (permissions{vote: true, withdraw: true}) {
		t.Errorf("proposer after their own ballot = %+v, want vote and withdraw", got)
	}
	d.ballot(t, p, other, "no")
	got := d.get(t, p, d.caller)
	if can(got) != (permissions{vote: true}) || can(d.get(t, p, other)) != (permissions{vote: true}) {
		t.Errorf("after another ballot proposer = %+v, other = %+v, want vote only", can(got),
			can(d.get(t, p, other)))
	}
	byID := map[uuid.UUID]app.Voter{}
	for _, v := range got.Voters {
		byID[v.UserID.UUID()] = v
	}
	if mine, theirs := byID[d.caller], byID[other]; len(got.Voters) != 2 || mine.Choice != domain.ChoiceYes ||
		theirs.Choice != domain.ChoiceNo || !mine.CastAt.Equal(d.now) ||
		got.Voters[0].UserID.String() != min(d.caller.String(), other.String()) {
		t.Errorf("voters = %+v, want both ballots ordered by user id", got.Voters)
	}
	d.setStatus(t, p, string(domain.StatusPassed), "")
	if got := can(d.get(t, p, d.caller)); got != (permissions{}) {
		t.Errorf("proposer on a passed proposal = %+v, want neither", got)
	}
}

func TestProposals_Detail_LinkedSwap(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.proposedBy(t, d.caller)
	if got := d.get(t, p, d.caller); got.Swap != nil {
		t.Fatalf("Swap with none linked = %+v, want nil", got.Swap)
	}
	source := trading.Source{Kind: "proposal", ID: p}
	first := trading.SwapView{
		ID: ids.SwapIDFrom(d.ids.NewV7()), Source: source, Status: "failed", FailureCode: "jupiter_failed",
		CreatedAt: d.now,
	}
	retry := trading.SwapView{
		ID: ids.SwapIDFrom(d.ids.NewV7()), Source: source, Status: "confirmed", TxSignature: "sig-1",
		CreatedAt: d.now.Add(time.Minute),
	}
	d.swaps.Put(first)
	d.swaps.Put(trading.SwapView{ID: ids.SwapIDFrom(d.ids.NewV7()), Source: trading.Source{Kind: "cashout", ID: p}})
	if got := d.get(t, p, d.caller); got.Swap == nil || *got.Swap != first {
		t.Fatalf("Swap after one failure = %+v, want %+v", got.Swap, first)
	}
	d.swaps.Put(retry)
	if got := d.get(t, p, d.caller); got.Swap == nil || *got.Swap != retry {
		t.Fatalf("Swap after the retry = %+v, want %+v", got.Swap, retry)
	}
}

func TestProposals_Detail_failures(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.proposedBy(t, d.caller)
	req := app.GetProposal{ID: ids.ProposalIDFrom(p), Caller: ids.UserIDFrom(d.caller)}
	unknown := app.GetProposal{ID: ids.ProposalIDFrom(d.ids.NewV7()), Caller: req.Caller}
	if _, err := d.reads().Get(t.Context(), unknown); errs.CodeOf(err) != errs.CodeProposalNotFound {
		t.Errorf("Get of an unknown proposal err = %v, want proposal_not_found", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.reads().Get(ctx, req); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("Get on a cancelled context err = %v, want internal", err)
	}
	d.swaps.FailOnce(errs.New(errs.CodeDBUnavailable, "t"))
	if _, err := d.reads().Get(t.Context(), req); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Errorf("Get whose swap read fails err = %v, want db_unavailable", err)
	}
	d.setStatus(t, p, "gremlins", "")
	if _, err := d.reads().Get(t.Context(), req); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Errorf("Get of an unknown stored status err = %v, want decode_failed", err)
	}
	if _, err := d.pool.Exec(t.Context(), `ALTER TABLE proposal_voters RENAME TO proposal_voters_gone`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.reads().Get(t.Context(), req); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("Get whose voter read fails err = %v, want internal", err)
	}
}

func TestHTTP_GetProposal(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	other := d.ids.NewV7()
	p := d.proposedBy(t, d.caller, other)
	d.ballot(t, p, other, "no")
	failed := trading.SwapView{
		ID: ids.SwapIDFrom(d.ids.NewV7()), Source: trading.Source{Kind: "proposal", ID: p}, Status: "failed",
		FailureCode: "jupiter_failed", Retryable: true, CreatedAt: d.now,
	}
	d.swaps.Put(failed)
	h := adapters.HTTP{Reads: d.reads()}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
	res, err := h.GetProposal(ctx, api.GetProposalRequestObject{Id: p})
	got, ok := res.(api.GetProposal200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("GetProposal = %#v, %v", res, err)
	}
	checkVoters(t, got.Voters, map[uuid.UUID]api.ProposalVoter{
		d.caller: {UserId: d.caller},
		other:    {UserId: other, Choice: ptr(api.BallotChoice("no")), CastAt: ptr(d.now)},
	})
	wantSwap := api.LinkedSwap{
		SwapId: failed.ID.UUID(), Status: "failed", FailureCode: ptr("jupiter_failed"),
		FailureMessage: ptr(errs.Message(errs.CodeSwapFailed)), Retryable: true,
	}
	type summary struct {
		id             uuid.UUID
		vote, withdraw bool
		tally          api.Tally
		swap           *api.LinkedSwap
	}
	failedSummary := summary{p, true, false, api.Tally{No: 1, Voters: 2, Needed: 2}, &wantSwap}
	if s := (summary{got.Id, got.CanVote, got.CanWithdraw, got.Tally, got.Swap}); !reflect.DeepEqual(s, failedSummary) {
		t.Errorf("detail = %+v, want %+v", s, failedSummary)
	}
	d.swaps.Put(trading.SwapView{
		ID: ids.SwapIDFrom(d.ids.NewV7()), Source: failed.Source, Status: "confirmed", TxSignature: "sig-1",
		CreatedAt: d.now.Add(time.Minute),
	})
	res, err = h.GetProposal(ctx, api.GetProposalRequestObject{Id: p})
	retried, ok := res.(api.GetProposal200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("GetProposal after a confirmed retry = %#v, %v", res, err)
	}
	wantRetry := api.LinkedSwap{SwapId: retried.Swap.SwapId, Status: "confirmed", TxSignature: ptr("sig-1")}
	if !reflect.DeepEqual(*retried.Swap, wantRetry) {
		t.Errorf("swap after a confirmed retry = %+v, want %+v", *retried.Swap, wantRetry)
	}
}

func checkVoters(t *testing.T, got []api.ProposalVoter, want map[uuid.UUID]api.ProposalVoter) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("voters = %+v, want %d", got, len(want))
	}
	for _, v := range got {
		w := want[v.UserId]
		if v.CastAt != nil && w.CastAt != nil && v.CastAt.Equal(*w.CastAt) {
			w.CastAt = v.CastAt
		}
		if !reflect.DeepEqual(v, w) {
			t.Errorf("voter = %+v, want %+v", v, w)
		}
	}
}

func TestHTTP_GetProposal_refusals(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.proposedBy(t, d.caller)
	h := adapters.HTTP{Reads: d.reads()}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
	if _, err := h.GetProposal(t.Context(), api.GetProposalRequestObject{Id: p}); errs.CodeOf(
		err) != errs.CodeUnauthorized {
		t.Errorf("GetProposal with no actor err = %v, want unauthorized", err)
	}
	if _, err := h.GetProposal(ctx, api.GetProposalRequestObject{Id: d.ids.NewV7()}); errs.CodeOf(
		err) != errs.CodeProposalNotFound {
		t.Errorf("GetProposal of an unknown proposal err = %v, want proposal_not_found", err)
	}
}
