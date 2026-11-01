package governance_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func (d memberRetry) operator(t *testing.T) context.Context {
	t.Helper()
	return observability.WithActor(t.Context(), "admin:operator")
}

func (d memberRetry) handler(swaps app.Swaps) *app.RetryBlockedTradeHandler {
	clk := testkit.NewClock(d.now)
	return app.NewRetryBlockedTradeHandler(
		db.New(d.pool, d.ids, clk), d.pool, clk, swaps, app.NoHints{},
		chain.SolanaAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"),
	)
}

func TestRetryBlockedTrade_refusesABlankReasonAndAnUnknownProposal(t *testing.T) {
	t.Parallel()
	d := newMemberRetry(t)
	id := d.blocked(t, "trade_not_started")
	h := d.handler(d.swaps)
	if err := h.Handle(
		t.Context(),
		app.RetryBlockedTrade{ProposalID: ids.ProposalIDFrom(id), Reason: " "},
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidInput {
		t.Fatalf("blank reason = %v, want invalid_input", err)
	}
	unknown := app.RetryBlockedTrade{ProposalID: ids.ProposalIDFrom(d.ids.NewV7()), Reason: "again"}
	if err := h.Handle(d.operator(t), unknown); errs.CodeOf(err) != errs.CodeProposalNotFound {
		t.Fatalf("unknown proposal = %v, want proposal_not_found", err)
	}
}

func TestRetryBlockedTrade_surfacesStoreFailuresWithoutRetrying(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		statement string
		want      errs.Code
	}{
		"the reopen":         {`ALTER TABLE proposals RENAME TO proposals_gone`, errs.CodeInternal},
		"the reopened event": {`ALTER TABLE events RENAME TO events_gone`, errs.CodeInternal},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newMemberRetry(t)
			id := d.blocked(t, "trade_not_started")
			if _, err := d.pool.Exec(t.Context(), tc.statement); err != nil {
				t.Fatal(err)
			}
			cmd := app.RetryBlockedTrade{ProposalID: ids.ProposalIDFrom(id), Reason: "again"}
			if err := d.handler(d.swaps).Handle(d.operator(t), cmd); errs.CodeOf(err) != tc.want {
				t.Fatalf("retry = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestRetryBlockedTrade_aFailedSwapLookupRefusesTheRetry(t *testing.T) {
	t.Parallel()
	d := newMemberRetry(t)
	id := d.blocked(t, "trade_not_started")
	swaps := fakes.NewTrading()
	swaps.Fail(errs.New(errs.CodeDBUnavailable, "t"))
	cmd := app.RetryBlockedTrade{ProposalID: ids.ProposalIDFrom(id), Reason: "again"}
	if err := d.handler(swaps).Handle(d.operator(t), cmd); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("retry = %v, want db_unavailable", err)
	}
	if n := d.count(t, id, events.TypeProposalReopened); n != 0 {
		t.Fatalf("proposal.reopened events = %d, want none", n)
	}
}

func TestRetryBlockedTrade_aSellRetriesTheTokensForUSDC(t *testing.T) {
	t.Parallel()
	d := newMemberRetry(t)
	id := d.blocked(t, "trade_not_started")
	if _, err := d.pool.Exec(t.Context(),
		`UPDATE proposals SET kind = 'sell', token_amount = 5, usdc_micros = NULL WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	cmd := app.RetryBlockedTrade{ProposalID: ids.ProposalIDFrom(id), Reason: "again"}
	if err := d.handler(d.swaps).Handle(d.operator(t), cmd); err != nil {
		t.Fatalf("retry = %v, want nil", err)
	}
	var action, inAmount string
	if err := d.pool.QueryRow(t.Context(),
		`SELECT payload->>'action', payload->>'in_amount' FROM events WHERE type = $1`,
		string(events.TypeTradeRetryRequested)).Scan(&action, &inAmount); err != nil {
		t.Fatal(err)
	}
	if action != "sell" || inAmount != "5" {
		t.Fatalf("trade.retry_requested = %s of %s, want a sell of the 5 tokens", action, inAmount)
	}
}
