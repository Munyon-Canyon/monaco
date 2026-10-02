package trading_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
)

type venueStub struct {
	app.Venue
	execute executeFunc
}

type executeFunc func(ctx context.Context, requestID string, signed []byte) (app.ExecuteResult, error)

func (v venueStub) ExecuteUntilTerminal(
	ctx context.Context,
	requestID string,
	signed []byte,
) (app.ExecuteResult, error) {
	return v.execute(ctx, requestID, signed)
}

func (e *layerEnv) stubExecute(fn executeFunc) {
	e.venue = venueStub{Venue: e.venue, execute: fn}
}

func (e *layerEnv) settled(t *testing.T, req app.SwapRequest) app.SwapView {
	t.Helper()
	got, err := e.run(t, req)
	if err != nil {
		t.Fatal(err)
	}
	e.assertAtMostOneTerminalEvent(t)
	return got
}

func TestSwapLayer_Success(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusSuccess, OutAmount: 105_000_000})
	req := e.request(e.source())
	got := e.settled(t, req)
	if got.Status != domain.StatusConfirmed || got.OutAmount != 105_000_000 || got.ConfirmedAt.IsZero() {
		t.Fatalf("view = %+v", got)
	}
	if want := []string{"trade.submitted", "trade.confirmed"}; !slices.Equal(e.events(t, got.ID.UUID()), want) {
		t.Fatalf("events = %v, want %v", e.events(t, got.ID.UUID()), want)
	}
	p := e.payload(t, got.ID.UUID(), "trade.confirmed")
	if p["out_amount"] != "105000000" || p["in_amount"] != "25000000" || p["tx_signature"] != string(got.TxSignature) {
		t.Fatalf("trade.confirmed payload = %v", p)
	}
	if p["usdc_micros"] != "25000000" {
		t.Fatalf("a buy spends its in amount: usdc_micros = %v", p["usdc_micros"])
	}
	if keys := e.hints.published(); len(keys) != 3 {
		t.Fatalf("hints = %v, want created, submitted and confirmed", keys)
	}
}

func TestSwapLayer_SellCountsTheOutAmountAsUSDC(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.jup.SetOrder(jupiterMint(aaplxToken()), jupiterMint(usdcToken()), jupiter.Order{
		RequestID: "req-sell", Transaction: e.treasuryTx(),
	})
	e.jup.SetExecute("req-sell", jupiter.ExecuteResult{Status: jupiter.StatusSuccess, OutAmount: 31_000_000})
	req := e.request(e.source())
	req.Action, req.InMint, req.OutMint, req.InAmount = domain.ActionSell, aaplxToken(), usdcToken(), 10_000_000
	got := e.settled(t, req)
	p := e.payload(t, got.ID.UUID(), "trade.confirmed")
	if got.Status != domain.StatusConfirmed ||
		p["usdc_micros"] != "31000000" {
		t.Fatalf("view %+v payload %v, want usdc_micros 31000000", got, p)
	}
}

func TestSwapLayer_Failed(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusFailed, ErrorCode: -1002})
	got := e.settled(t, e.request(e.source()))
	if got.Status != domain.StatusFailed || got.FailureCode != domain.FailureJupiterFailed {
		t.Fatalf("view = %+v", got)
	}
	if want := []string{"trade.submitted", "trade.failed"}; !slices.Equal(e.events(t, got.ID.UUID()), want) {
		t.Fatalf("events = %v, want %v", e.events(t, got.ID.UUID()), want)
	}
	p := e.payload(t, got.ID.UUID(), "trade.failed")
	if p["failure_code"] != "jupiter_failed" || p["jupiter_code"] != "-1002" {
		t.Fatalf("trade.failed payload = %v", p)
	}
}

func TestSwapLayer_PendingTwiceThenSuccess(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.jup.SetExecute("req-1",
		jupiter.ExecuteResult{Status: jupiter.StatusPending}, jupiter.ExecuteResult{Status: jupiter.StatusPending},
		jupiter.ExecuteResult{Status: jupiter.StatusSuccess, OutAmount: 105_000_000})
	got := e.settled(t, e.request(e.source()))
	if got.Status != domain.StatusConfirmed {
		t.Fatalf("view = %+v", got)
	}
	sent := e.jup.Sent("req-1")
	if len(sent) != 3 {
		t.Fatalf("%d sends, want 3", len(sent))
	}
	for _, b := range sent {
		if !slices.Equal(b, sent[0]) {
			t.Fatal("a re-send changed the signed bytes")
		}
	}
}

func TestSwapLayer_PendingTimeout_LeavesSubmitted(t *testing.T) {
	t.Parallel()
	for name, arm := range map[string]func(*layerEnv){
		"still pending after the window": func(e *layerEnv) {
			e.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusPending})
		},
		"jupiter down after submit": func(e *layerEnv) {
			e.jup.Fail("Execute", errs.New(errs.CodeJupiterUnavailable, "test"))
		},
		"pending without an error": func(e *layerEnv) {
			e.stubExecute(func(context.Context, string, []byte) (app.ExecuteResult, error) {
				return app.ExecuteResult{Status: app.ExecutePending}, nil
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newLayerEnv(t)
			arm(e)
			got := e.settled(t, e.request(e.source()))
			if got.Status != domain.StatusSubmitted || e.terminalEvents(t, got.ID.UUID()) != 0 {
				t.Fatalf("view = %+v, want submitted with no terminal event", got)
			}
		})
	}
}

func TestSwapLayer_ExecuteErrorLeavesSubmittedAndSurfaces(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.jup.Fail("Execute", errs.New(errs.CodeInternal, "test"))
	req := e.request(e.source())
	if _, err := e.run(t, req); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Run = %v, want internal", err)
	}
	rows := e.swapIDs(t, req.Source.ID)
	if status, _, _, _ := e.row(t, rows[0]); status != "submitted" {
		t.Fatalf("status = %s, want submitted", status)
	}
}

func TestSwapLayer_SignedBytesCommittedBeforeExecute(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	called := false
	inner := e.venue
	e.stubExecute(func(ctx context.Context, requestID string, signed []byte) (app.ExecuteResult, error) {
		called = true
		var status string
		var stored []byte
		var request string
		if err := e.pool.QueryRow(ctx, `SELECT status, signed_tx, execute_request_id FROM swaps`).
			Scan(&status, &stored, &request); err != nil {
			t.Error(err)
		}
		if status != "submitted" || len(stored) == 0 || !slices.Equal(stored, signed) || request != requestID {
			t.Errorf("at /execute the row is %s with %d stored bytes (request %q), want submitted with the sent bytes",
				status, len(stored), request)
		}
		return inner.ExecuteUntilTerminal(ctx, requestID, signed)
	})
	e.settled(t, e.request(e.source()))
	if !called {
		t.Fatal("/execute was never called")
	}
}

func TestSwapLayer_TerminalWriteLosingTheRaceReturnsTheCurrentRowWithoutAnEvent(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	inner := e.venue
	e.stubExecute(func(ctx context.Context, requestID string, signed []byte) (app.ExecuteResult, error) {
		if _, err := e.pool.Exec(
			ctx,
			`UPDATE swaps SET status = 'failed', failure_code = 'force_resolved', failed_at = now()`,
		); err != nil {
			t.Error(err)
		}
		return inner.ExecuteUntilTerminal(ctx, requestID, signed)
	})
	e.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusSuccess, OutAmount: 1})
	got := e.settled(t, e.request(e.source()))
	if got.Status != domain.StatusFailed || got.FailureCode != domain.FailureForceResolved {
		t.Fatalf("view = %+v, want the row the other path wrote", got)
	}
	if e.terminalEvents(t, got.ID.UUID()) != 0 {
		t.Fatal("the loser appended a terminal event")
	}
}

func TestSwapLayer_RefusesAnOutAmountTheColumnCannotHold(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.stubExecute(func(context.Context, string, []byte) (app.ExecuteResult, error) {
		return app.ExecuteResult{Status: app.ExecuteSuccess, OutAmount: 1 << 63}, nil
	})
	req := e.request(e.source())
	if _, err := e.run(t, req); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Run = %v, want decode_failed", err)
	}
	if e.terminalEvents(t, e.swapIDs(t, req.Source.ID)[0]) != 0 {
		t.Fatal("a refused amount produced a terminal event")
	}
}

func (e *layerEnv) terminalEvents(t *testing.T, swap uuid.UUID) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE aggregate_id = $1
		AND type IN ('trade.confirmed', 'trade.failed')`, swap).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *layerEnv) treasuryTx() []byte {
	return chainfake.Unsigned(chainfake.WalletAddress(treasuryWallet))
}
