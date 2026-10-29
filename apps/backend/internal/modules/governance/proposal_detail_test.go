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

func (d readsDB) setMint(t *testing.T, id uuid.UUID, mint string) {
	t.Helper()
	if _, err := d.pool.Exec(t.Context(), `UPDATE proposals SET mint = $2 WHERE id = $1`, id, mint); err != nil {
		t.Fatal(err)
	}
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
	checkOrderedBallots(t, got.Voters, d.caller, other, d.now)
	d.setStatus(t, p, string(domain.StatusPassed), "")
	if got := can(d.get(t, p, d.caller)); got != (permissions{}) {
		t.Errorf("proposer on a passed proposal = %+v, want neither", got)
	}
}

func checkOrderedBallots(t *testing.T, voters []app.Voter, caller, other uuid.UUID, at time.Time) {
	t.Helper()
	byID := map[uuid.UUID]app.Voter{}
	for _, v := range voters {
		byID[v.UserID.UUID()] = v
	}
	if mine, theirs := byID[caller], byID[other]; len(voters) != 2 || mine.Choice != domain.ChoiceYes ||
		theirs.Choice != "" || !theirs.Voted || !mine.CastAt.Equal(at) ||
		voters[0].UserID.String() != min(caller.String(), other.String()) {
		t.Errorf("voters = %+v, want the caller's side, the other voter's side private, ordered by user id", voters)
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
	d.setMint(t, p, "not-a-mint")
	d.swaps.Put(trading.SwapView{ID: ids.SwapIDFrom(d.ids.NewV7()), Source: trading.Source{Kind: "proposal", ID: p}})
	if _, err := d.reads().Get(t.Context(), req); err == nil {
		t.Error("Get of a proposal with a malformed mint err = nil, want an error")
	}
	d.setMint(t, p, aaplxMint)
	d.assets.Fail("AssetByMint", errs.New(errs.CodeDBUnavailable, "t"))
	d.swaps.Put(trading.SwapView{ID: ids.SwapIDFrom(d.ids.NewV7()), Source: trading.Source{Kind: "proposal", ID: p}})
	if _, err := d.reads().Get(t.Context(), req); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Errorf("Get whose asset read fails err = %v, want db_unavailable", err)
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

func TestHTTP_GetProposal_neverSubmittedSwapSaysTheTradeWasNotSent(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.proposedBy(t, d.caller, d.ids.NewV7())
	d.swaps.Put(trading.SwapView{
		ID: ids.SwapIDFrom(d.ids.NewV7()), Source: trading.Source{Kind: "proposal", ID: p}, Status: "failed",
		FailureCode: "never_submitted", Retryable: true, CreatedAt: d.now,
	})
	d.setStatus(t, p, string(domain.StatusExecutionBlocked), string(errs.CodeSwapFailed))
	h := adapters.HTTP{Reads: d.reads()}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
	res, err := h.GetProposal(ctx, api.GetProposalRequestObject{Id: p})
	got, ok := res.(api.GetProposal200JSONResponse)
	if err != nil || !ok || got.Swap == nil || got.Swap.FailureMessage == nil {
		t.Fatalf("GetProposal = %#v, %v, want a swap with a failure message", res, err)
	}
	if want := errs.Message(errs.CodeSwapNotSent); *got.Swap.FailureMessage != want {
		t.Errorf("failure message = %q, want %q", *got.Swap.FailureMessage, want)
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
	d.setStatus(t, p, string(domain.StatusExecutionBlocked), string(errs.CodeSwapFailed))
	h := adapters.HTTP{Reads: d.reads()}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
	res, err := h.GetProposal(ctx, api.GetProposalRequestObject{Id: p})
	got, ok := res.(api.GetProposal200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("GetProposal = %#v, %v", res, err)
	}
	checkVoters(t, got.Voters, map[uuid.UUID]api.ProposalVoter{
		d.caller: {UserId: d.caller},
		other:    {UserId: other, Voted: true},
	})
	wantSwap := api.LinkedSwap{
		SwapId: failed.ID.UUID(), Status: "failed", FailureCode: ptr("jupiter_failed"),
		FailureMessage: ptr(errs.Message(errs.CodeSwapFailed)), Retryable: true, TokenDecimals: 8,
	}
	type summary struct {
		id             uuid.UUID
		vote, withdraw bool
		tally          api.Tally
		swap           *api.LinkedSwap
	}
	failedSummary := summary{p, false, false, api.Tally{No: 1, Voters: 2, Needed: 2}, &wantSwap}
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
	wantRetry := api.LinkedSwap{
		SwapId: retried.Swap.SwapId, Status: "confirmed", TxSignature: ptr("sig-1"), TokenDecimals: 8,
	}
	if !reflect.DeepEqual(*retried.Swap, wantRetry) {
		t.Errorf("swap after a confirmed retry = %+v, want %+v", *retried.Swap, wantRetry)
	}
}

func TestHTTP_GetProposal_retryOnlyWhenTheEngineCanRetry(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, status, reason string
		retryable            bool
	}{
		{"swap failed", "execution_blocked", string(errs.CodeSwapFailed), true},
		{"passed", "passed", "", true},
		{"retry refused", "execution_blocked", "slippage_exceeded", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := newReadsDB(t)
			p := d.proposedBy(t, d.caller)
			d.swaps.Put(trading.SwapView{
				ID: ids.SwapIDFrom(d.ids.NewV7()), Source: trading.Source{Kind: "proposal", ID: p}, Status: "failed",
				FailureCode: "jupiter_failed", Retryable: true, CreatedAt: d.now,
			})
			d.setStatus(t, p, tc.status, tc.reason)
			h := adapters.HTTP{Reads: d.reads()}
			ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
			res, err := h.GetProposal(ctx, api.GetProposalRequestObject{Id: p})
			got, ok := res.(api.GetProposal200JSONResponse)
			if err != nil || !ok {
				t.Fatalf("GetProposal = %#v, %v", res, err)
			}
			if got.Swap.Retryable != tc.retryable || (got.Swap.FailureMessage != nil) != tc.retryable {
				t.Errorf("retryable = %v, failureMessage = %v, want retryable %v with a message only then",
					got.Swap.Retryable, got.Swap.FailureMessage, tc.retryable)
			}
		})
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

func TestHTTP_GetProposal_swapFill(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		swap trading.SwapView
		want api.LinkedSwap
	}{
		"confirmed buy": {
			trading.SwapView{Action: "buy", Status: "confirmed", InAmount: 25_000_000, OutAmount: 12_500_000},
			api.LinkedSwap{
				UsdcMicros: ptr(int64(25_000_000)), TokenAmount: ptr(int64(12_500_000)), PriceMicros: ptr(int64(200_000_000)),
			},
		},
		"confirmed sell floors the price": {
			trading.SwapView{Action: "sell", Status: "confirmed", InAmount: 3_000_000_000, OutAmount: 10_000_000},
			api.LinkedSwap{
				UsdcMicros: ptr(int64(10_000_000)), TokenAmount: ptr(int64(3_000_000_000)), PriceMicros: ptr(int64(333_333)),
			},
		},
		"submitted buy": {
			trading.SwapView{Action: "buy", Status: "submitted", InAmount: 25_000_000},
			api.LinkedSwap{UsdcMicros: ptr(int64(25_000_000))},
		},
		"failed sell": {
			trading.SwapView{Action: "sell", Status: "failed", InAmount: 3_000_000_000, FailureCode: "jupiter_failed"},
			api.LinkedSwap{TokenAmount: ptr(int64(3_000_000_000)), FailureCode: ptr("jupiter_failed")},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newReadsDB(t)
			p := d.proposedBy(t, d.caller)
			tc.swap.ID, tc.swap.CreatedAt = ids.SwapIDFrom(d.ids.NewV7()), d.now
			tc.swap.Source = trading.Source{Kind: "proposal", ID: p}
			d.swaps.Put(tc.swap)
			ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
			res, err := adapters.HTTP{Reads: d.reads()}.GetProposal(ctx, api.GetProposalRequestObject{Id: p})
			got, ok := res.(api.GetProposal200JSONResponse)
			if err != nil || !ok || got.Swap == nil {
				t.Fatalf("GetProposal = %#v, %v, want a swap", res, err)
			}
			tc.want.SwapId, tc.want.TokenDecimals = tc.swap.ID.UUID(), 8
			tc.want.Status = api.ProposalDetailSwapStatus(tc.swap.Status)
			if !reflect.DeepEqual(*got.Swap, tc.want) {
				t.Errorf("swap = %+v, want %+v", *got.Swap, tc.want)
			}
		})
	}
}
