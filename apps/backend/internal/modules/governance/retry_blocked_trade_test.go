package governance_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type memberRetry struct {
	readsDB
	roster *cabalRoster
	http   adapters.HTTP
}

func newMemberRetry(t *testing.T) memberRetry {
	t.Helper()
	d := newReadsDB(t)
	roster := d.roster()
	clk := testkit.NewClock(d.now)
	retry := app.NewRetryBlockedTradeHandler(
		db.New(d.pool, d.ids, clk), d.pool, clk, d.swaps, app.NoHints{},
		chain.SolanaAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"),
	).Members(roster)
	return memberRetry{readsDB: d, roster: roster, http: adapters.HTTP{RetryTrade: retry, Reads: d.reads()}}
}

func (d memberRetry) blocked(t *testing.T, code string) uuid.UUID {
	t.Helper()
	id := d.proposedBy(t, d.caller)
	d.setStatus(t, id, "execution_blocked", code)
	return id
}

func (d memberRetry) as(user uuid.UUID) auth.Actor {
	return auth.Actor{Kind: auth.ActorUser, ID: user.String()}
}

func (d memberRetry) retry(t *testing.T, user, proposal uuid.UUID) (api.PostProposalRetryTradeResponseObject, error) {
	t.Helper()
	return d.http.PostProposalRetryTrade(
		auth.WithActor(t.Context(), d.as(user)), api.PostProposalRetryTradeRequestObject{Id: proposal},
	)
}

func (d memberRetry) count(t *testing.T, proposal uuid.UUID, typ events.Type) int {
	t.Helper()
	var n int
	if err := d.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM events WHERE type = $2 AND (aggregate_id = $1 OR payload->'source'->>'id' = $1::text)`,
		proposal, string(typ)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestHTTP_PostProposalRetryTrade_aMemberReopensAProposalBlockedBeforeAnySwap(t *testing.T) {
	t.Parallel()
	d := newMemberRetry(t)
	id := d.blocked(t, "trade_not_started")
	got, err := d.retry(t, d.caller, id)
	if err != nil {
		t.Fatalf("PostProposalRetryTrade = %v, want nil", err)
	}
	out, ok := got.(api.PostProposalRetryTrade200JSONResponse)
	if !ok || out.Status != api.ProposalStatus("passed") || out.StatusReason != nil {
		t.Fatalf("response = %#v, want the proposal passed again with no reason", got)
	}
	var actor, reason string
	if err := d.pool.QueryRow(t.Context(),
		`SELECT actor_type || ':' || actor_id, payload->>'reason' FROM events
		 WHERE type = 'proposal.reopened' AND aggregate_id = $1`, id).Scan(&actor, &reason); err != nil {
		t.Fatal(err)
	}
	if want := "user:" + d.caller.String(); actor != want || reason != "member retry" {
		t.Fatalf("proposal.reopened by %q for %q, want %q for member retry", actor, reason, want)
	}
	if n := d.count(t, id, events.TypeTradeRetryRequested); n != 1 {
		t.Fatalf("trade.retry_requested events = %d, want 1", n)
	}
}

func TestHTTP_PostProposalRetryTrade_refusesWhatItCannotRetry(t *testing.T) {
	t.Parallel()
	d := newMemberRetry(t)
	stranger := d.ids.NewV7()
	aborted := d.blocked(t, "no_route")
	withSwap := d.blocked(t, "trade_not_started")
	d.swaps.Put(trading.SwapView{
		ID: ids.SwapIDFrom(d.ids.NewV7()), Source: trading.Source{Kind: "proposal", ID: withSwap},
		CreatedAt: d.now.Add(time.Minute),
	})
	notStarted := d.blocked(t, "trade_not_started")
	d.roster.leave(d.cabal, stranger)
	for _, tc := range []struct {
		name     string
		user, id uuid.UUID
		want     errs.Code
	}{
		{"a non-member", stranger, notStarted, errs.CodeNotCabalMember},
		{"a different block code", d.caller, aborted, errs.CodeSwapNotRetryable},
		{"a proposal that already has a swap", d.caller, withSwap, errs.CodeSwapNotRetryable},
		{"an unknown proposal", d.caller, d.ids.NewV7(), errs.CodeProposalNotFound},
	} {
		if _, err := d.retry(t, tc.user, tc.id); errs.CodeOf(err) != tc.want {
			t.Errorf("retry by %s = %v, want %s", tc.name, err, tc.want)
		}
	}
	for _, id := range []uuid.UUID{notStarted, aborted, withSwap} {
		if n := d.count(t, id, events.TypeProposalReopened) + d.count(t, id, events.TypeTradeRetryRequested); n != 0 {
			t.Errorf("refused retries appended %d events for %s, want none", n, id)
		}
	}
	if r := d.row(t, notStarted); r.status.String != "execution_blocked" {
		t.Errorf("a refused retry left the proposal %s, want execution_blocked", r.status.String)
	}
	anonymous := api.PostProposalRetryTradeRequestObject{Id: notStarted}
	if _, err := d.http.PostProposalRetryTrade(t.Context(), anonymous); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Errorf("retry with no actor = %v, want unauthorized", err)
	}
}

func TestHTTP_PostProposalRetryTrade_aSecondRetryIsRefused(t *testing.T) {
	t.Parallel()
	d := newMemberRetry(t)
	id := d.blocked(t, "trade_not_started")
	if _, err := d.retry(t, d.caller, id); err != nil {
		t.Fatalf("first retry = %v, want nil", err)
	}
	if _, err := d.retry(t, d.caller, id); errs.CodeOf(err) != errs.CodeSwapNotRetryable {
		t.Fatalf("second retry = %v, want swap_not_retryable", err)
	}
	if n := d.count(t, id, events.TypeTradeRetryRequested); n != 1 {
		t.Fatalf("trade.retry_requested events after two retries = %d, want 1", n)
	}
}

func TestHTTP_PostProposalRetryTrade_aRosterFailureRefusesTheRetry(t *testing.T) {
	t.Parallel()
	d := newMemberRetry(t)
	id := d.blocked(t, "trade_not_started")
	d.roster.err = errs.New(errs.CodeDBUnavailable, "t")
	if _, err := d.retry(t, d.caller, id); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("retry whose membership check fails = %v, want db_unavailable", err)
	}
	if n := d.count(t, id, events.TypeProposalReopened); n != 0 {
		t.Fatalf("proposal.reopened events = %d, want none", n)
	}
}

func TestHTTP_PostProposalRetryTrade_aFailedDetailReadStillLeavesTheRetryRequested(t *testing.T) {
	t.Parallel()
	d := newMemberRetry(t)
	id := d.blocked(t, "trade_not_started")
	broken := d.readsDB
	broken.swaps = fakes.NewTrading()
	broken.swaps.Fail(errs.New(errs.CodeDBUnavailable, "t"))
	d.http.Reads = broken.reads()
	if _, err := d.retry(t, d.caller, id); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("retry whose detail read fails = %v, want db_unavailable", err)
	}
	if n := d.count(t, id, events.TypeTradeRetryRequested); n != 1 {
		t.Fatalf("trade.retry_requested events = %d, want 1", n)
	}
}
