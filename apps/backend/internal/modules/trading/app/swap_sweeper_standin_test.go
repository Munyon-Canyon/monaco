package app

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (e *sweeperEnv) submitStandIn(t *testing.T) (sqlc.InsertCreatedParams, chain.Signature) {
	t.Helper()
	p := e.insert(t)
	standIn := chain.StandInSignature("treasury-" + chain.Signature(p.ID.String()))
	params := sqlc.MarkSubmittedParams{
		ID: p.ID, ExecuteRequestID: p.ID.String(), SignedTx: []byte{1}, TxSignature: string(standIn),
		SubmittedAt: e.clk.Now(),
	}
	if n, err := e.q.MarkSubmitted(t.Context(), params); err != nil || n != 1 {
		t.Fatalf("MarkSubmitted = %d, %v", n, err)
	}
	return p, standIn
}

func (e *sweeperEnv) stored(t *testing.T, id uuid.UUID) (signature string, parked *string) {
	t.Helper()
	if err := e.pool.QueryRow(t.Context(),
		`SELECT tx_signature, parked_code FROM swaps WHERE id = $1`, id).Scan(&signature, &parked); err != nil {
		t.Fatal(err)
	}
	return signature, parked
}

func (e *sweeperEnv) tickLogged(t *testing.T) (*testkit.Logs, error) {
	t.Helper()
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(tickContext(t), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	_, err := e.poller().Tick(ctx)
	return logs, err
}

func TestSwapSweeper_adoptsTheRealSignatureOfAStandInAndSettlesTheSwap(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row, standIn := e.submitStandIn(t)
	e.read.found = "real-signature"
	e.read.statuses = []SigStatus{{State: SigFinalized}}
	e.advance()
	if _, err := e.tickLogged(t); err != nil {
		t.Fatal(err)
	}
	if e.status(t, row.ID) != "confirmed" || e.terminalEvents(t, row.ID) != 1 {
		t.Fatalf(
			"status = %s with %d terminal events, want confirmed once",
			e.status(t, row.ID),
			e.terminalEvents(t, row.ID),
		)
	}
	if signature, _ := e.stored(t, row.ID); signature != "real-signature" {
		t.Fatalf("stored tx_signature = %q, want the recovered one", signature)
	}
	if got, ok := e.terminalEvent(t, row.ID).(events.TradeConfirmed); !ok || got.TxSignature != "real-signature" {
		t.Fatalf("event = %#v, want trade.confirmed on the real signature", e.terminalEvent(t, row.ID))
	}
	if e.read.findOwner != "treasury" || e.read.findStandIn != standIn {
		t.Fatalf("searched %q for %q, want the swap's treasury and its stand-in", e.read.findOwner, e.read.findStandIn)
	}
	for _, batch := range e.read.batches {
		for _, sig := range batch {
			if chain.IsStandInSignature(sig) {
				t.Fatalf("asked the chain about the stand-in %q", sig)
			}
		}
	}
}

func TestSwapSweeper_aRecoveredStandInThatFailedOnChainIsBookedFailed(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row, _ := e.submitStandIn(t)
	e.read.found = "real-signature"
	e.read.statuses = []SigStatus{{State: SigFinalized, Failed: true}}
	e.advance()
	if _, err := e.tickLogged(t); err != nil {
		t.Fatal(err)
	}
	if e.status(t, row.ID) != "failed" {
		t.Fatalf("status = %s, want failed: the real transaction reverted", e.status(t, row.ID))
	}
}

func TestSwapSweeper_waitsOnAStandInWhileItsBlockhashIsStillValid(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row, standIn := e.submitStandIn(t)
	e.read.valid = true
	e.advance()
	if _, err := e.tickLogged(t); err != nil {
		t.Fatal(err)
	}
	signature, parked := e.stored(t, row.ID)
	if e.status(t, row.ID) != "submitted" || parked != nil || signature != string(standIn) {
		t.Fatalf("status %s, parked %v, signature %q; want untouched while it may still land",
			e.status(t, row.ID), parked, signature)
	}
	for _, batch := range e.read.batches {
		if len(batch) != 0 {
			t.Fatalf("the chain was asked about %v, want nothing for a stand-in", batch)
		}
	}
}

func TestSwapSweeper_parksAStandInItCannotRecoverOnceTheBlockhashExpired(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row, _ := e.submitStandIn(t)
	e.read.valid = false
	e.advance()
	logs, err := e.tickLogged(t)
	if err != nil {
		t.Fatal(err)
	}
	_, parked := e.stored(t, row.ID)
	if e.status(t, row.ID) != "submitted" || parked == nil || *parked != ParkedSwapSignatureUnknown ||
		e.terminalEvents(t, row.ID) != 0 {
		t.Fatalf("status %s, parked %v, terminal events %d; want parked as submitted and never failed",
			e.status(t, row.ID), parked, e.terminalEvents(t, row.ID))
	}
	if len(logs.Lines(observability.TradingRoutedSwapParked.Name)) == 0 {
		t.Fatal("no alert for the parked swap")
	}
	e.advance()
	again, err := e.tickLogged(t)
	if alerts := len(again.Lines(observability.TradingRoutedSwapParked.Name)); err != nil || alerts != 0 {
		t.Fatalf("second sweep = %v with %d alerts; want one alert per park", err, alerts)
	}
}

func TestSwapSweeper_aParkedStandInIsAdoptedAndUnparkedWhenTheSignatureAppears(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row, _ := e.submitStandIn(t)
	e.advance()
	if _, err := e.tickLogged(t); err != nil {
		t.Fatal(err)
	}
	e.read.found = "late-signature"
	e.read.statuses = []SigStatus{{State: SigProcessing}}
	e.advance()
	if _, err := e.tickLogged(t); err != nil {
		t.Fatal(err)
	}
	signature, parked := e.stored(t, row.ID)
	if signature != "late-signature" || parked != nil || e.status(t, row.ID) != "submitted" {
		t.Fatalf("signature %q, parked %v, status %s; want adopted, unparked, still processing",
			signature, parked, e.status(t, row.ID))
	}
}

func TestSwapSweeper_aSearchFailureLeavesTheStandInAloneAndIsReported(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row, standIn := e.submitStandIn(t)
	e.read.findErr = errs.New(errs.CodeRPCUnavailable, "test")
	e.advance()
	if _, err := e.tickLogged(t); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("Tick = %v, want rpc_unavailable", err)
	}
	signature, parked := e.stored(t, row.ID)
	if e.status(t, row.ID) != "submitted" || parked != nil || signature != string(standIn) {
		t.Fatalf("status %s, parked %v, signature %q; want untouched", e.status(t, row.ID), parked, signature)
	}
}

func TestForceResolve_refusesToBookAStandInSwapWithoutAnOverride(t *testing.T) {
	t.Parallel()
	for _, to := range []domain.Status{domain.StatusFailed, domain.StatusConfirmed} {
		t.Run(string(to), func(t *testing.T) {
			t.Parallel()
			e := newSweeperEnv(t)
			e.read.statuses = []SigStatus{{State: SigNotFound}}
			row, standIn := e.submitStandIn(t)
			_, err := e.force().Handle(t.Context(), forceCommand(standIn, to))
			if errs.CodeOf(err) != errs.CodeInvalidInput {
				t.Fatalf("Handle = %v, want invalid_input without --override", err)
			}
			if e.read.calls != 0 || e.status(t, row.ID) != "submitted" {
				t.Fatalf(
					"chain calls %d, status %s; want no chain lookup of a stand-in and no change",
					e.read.calls,
					e.status(t, row.ID),
				)
			}
		})
	}
}

func TestSwapSweeper_aParkThatCannotBeWrittenIsReportedAndNothingIsFailed(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row, _ := e.submitStandIn(t)
	const refuse = `
		CREATE FUNCTION refuse_park() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'park refused'; END $$;
		CREATE TRIGGER refuse_park BEFORE UPDATE OF parked_at ON swaps
		FOR EACH ROW EXECUTE FUNCTION refuse_park()`
	if _, err := e.pool.Exec(t.Context(), refuse); err != nil {
		t.Fatal(err)
	}
	e.advance()
	if _, err := e.tickLogged(t); err == nil {
		t.Fatal("Tick = nil, want the park failure reported")
	}
	if _, parked := e.stored(t, row.ID); parked != nil || e.status(t, row.ID) != "submitted" {
		t.Fatalf("parked %v, status %s; want untouched", parked, e.status(t, row.ID))
	}
}
