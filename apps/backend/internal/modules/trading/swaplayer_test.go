package trading_test

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
)

func TestSwapLayer_RunSubmitsTheSignedTransaction(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusPending})
	req := e.request(e.source())
	got, err := e.run(t, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusSubmitted || got.Source != req.Source || got.InAmount != 25_000_000 {
		t.Fatalf("view = %+v", got)
	}
	status, signed, requestID, signature := e.row(t, got.ID.UUID())
	if status != "submitted" || len(signed) == 0 || requestID == nil || *requestID != "req-1" || signature == nil {
		t.Fatalf("row = %s, %d signed bytes, request %v, signature %v", status, len(signed), requestID, signature)
	}
	assertSignatureOf(t, got, signed, *signature)
	e.assertSubmittedEvent(t, got, *signature)
	e.assertHints(t, req, got, 2)
}

type orderLog struct {
	app.Venue
	mu    sync.Mutex
	specs []app.OrderSpec
}

func (o *orderLog) Order(ctx context.Context, spec app.OrderSpec) (app.Order, error) {
	o.mu.Lock()
	o.specs = append(o.specs, spec)
	o.mu.Unlock()
	return o.Venue.Order(ctx, spec)
}

func TestSwapLayer_TheRelayerPaysTheFeeAndCoSigns(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	orders := &orderLog{Venue: e.venue}
	e.venue = orders
	req := e.request(e.source())
	got, err := e.run(t, req)
	if err != nil {
		t.Fatal(err)
	}
	relayer := chainfake.RelayerAddress()
	want := []app.OrderSpec{{
		Taker: req.TreasuryWallet.Address, Payer: relayer, InMint: req.InMint, OutMint: req.OutMint,
		InAmount: req.InAmount, SlippageBps: req.SlippageBps,
	}}
	if !slices.Equal(orders.specs, want) {
		t.Fatalf("orders = %+v, want %+v", orders.specs, want)
	}
	tx := e.onlySent(t, "req-1")
	if tx.Signers[0] != relayer || tx.Signers[1] != req.TreasuryWallet.Address || !tx.Signed(0) || !tx.Signed(1) {
		t.Fatalf("sent signers %v signed (%v, %v), want the relayer and the treasury both signed",
			tx.Signers, tx.Signed(0), tx.Signed(1))
	}
	_, _, _, signature := e.row(t, got.ID.UUID())
	if signature == nil || platform.Signature(*signature) != platform.SignatureOf(tx.Signatures[0]) {
		t.Fatalf("stored tx_signature %v, want the relayer's signature 0", signature)
	}
}

func (e *layerEnv) onlySent(t *testing.T, requestID string) platform.Transaction {
	t.Helper()
	sent := e.jup.Sent(requestID)
	if len(sent) != 1 {
		t.Fatalf("%d transactions sent to execute, want 1", len(sent))
	}
	tx, err := platform.DecodeTransaction(sent[0])
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func assertSignatureOf(t *testing.T, got app.SwapView, signed []byte, signature string) {
	t.Helper()
	tx, err := platform.DecodeTransaction(signed)
	if err != nil || !tx.Signed(0) || signature != string(platform.SignatureOf(tx.Signatures[0])) {
		t.Fatalf("stored signature %q does not match the stored bytes (%v)", signature, err)
	}
	if got.TxSignature != platform.Signature(signature) {
		t.Fatalf("view signature = %q, row = %q", got.TxSignature, signature)
	}
}

func (e *layerEnv) assertSubmittedEvent(t *testing.T, got app.SwapView, signature string) {
	t.Helper()
	if want := []string{"trade.submitted"}; !slices.Equal(e.events(t, got.ID.UUID()), want) {
		t.Fatalf("events = %v, want %v", e.events(t, got.ID.UUID()), want)
	}
	payload := e.payload(t, got.ID.UUID(), "trade.submitted")
	if payload["tx_signature"] != signature || payload["in_amount"] != "25000000" || payload["symbol"] != "AAPLx" {
		t.Fatalf("trade.submitted payload = %v", payload)
	}
}

func (e *layerEnv) assertHints(t *testing.T, req app.SwapRequest, got app.SwapView, want int) {
	t.Helper()
	wantKey := "cabal." + req.CabalID.UUID().String() + ".swap_updated"
	keys := e.hints.published()
	if len(keys) != want || slices.ContainsFunc(keys, func(k string) bool { return k != wantKey }) {
		t.Fatalf("hints = %v, want %d on %s", keys, want, wantKey)
	}
	if body := string(e.hints.body[0]); body != `{"swap_id":"`+got.ID.UUID().String()+`"}` {
		t.Fatalf("hint payload = %s", body)
	}
}

func TestSwapLayer_DuplicateRun_OneSwap(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	req := e.request(e.source())
	var wg sync.WaitGroup
	views := make([]app.SwapView, 2)
	failures := make([]error, 2)
	for i := range views {
		wg.Go(func() { views[i], failures[i] = e.run(t, req) })
	}
	wg.Wait()
	if failures[0] != nil || failures[1] != nil {
		t.Fatalf("runs failed: %v, %v", failures[0], failures[1])
	}
	if views[0].ID != views[1].ID {
		t.Fatalf("two runs returned two swaps: %v and %v", views[0].ID, views[1].ID)
	}
	rows := e.swapIDs(t, req.Source.ID)
	if len(rows) != 1 {
		t.Fatalf("%d swap rows for one source, want 1", len(rows))
	}
	submitted := slices.DeleteFunc(e.events(t, rows[0]), func(typ string) bool { return typ != "trade.submitted" })
	if len(submitted) != 1 {
		t.Fatalf("%d trade.submitted events, want 1", len(submitted))
	}
}

func TestSwapLayer_RunAfterSubmitReturnsTheExistingSwap(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	req := e.request(e.source())
	first, err := e.run(t, req)
	if err != nil {
		t.Fatal(err)
	}
	again, err := e.run(t, req)
	if err != nil || again.ID != first.ID || len(e.swapIDs(t, req.Source.ID)) != 1 {
		t.Fatalf("second run = %+v, %v", again, err)
	}
}

func TestSwapLayer_OrderRejected(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.jup.Fail("Order", errs.New(errs.CodeJupiterRejected, "test"))
	req := e.request(e.source())
	got, err := e.run(t, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusFailed || got.FailureCode != domain.FailureNeverSubmitted {
		t.Fatalf("view = %+v, want failed never_submitted", got)
	}
	if want := []string{"trade.failed"}; !slices.Equal(e.events(t, got.ID.UUID()), want) {
		t.Fatalf("events = %v, want %v", e.events(t, got.ID.UUID()), want)
	}
	if code := e.payload(t, got.ID.UUID(), "trade.failed")["failure_code"]; code != "never_submitted" {
		t.Fatalf("failure_code = %v", code)
	}
	e.assertAtMostOneTerminalEvent(t)
	e.jup.Fail("Order", nil)
	retry, err := e.run(t, req)
	if err != nil || retry.ID == got.ID {
		t.Fatalf("a failed swap must free the claim: %+v, %v", retry, err)
	}
}

func TestSwapLayer_JupiterDownBeforeSubmit_LeavesCreated(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.jup.Fail("Order", errs.New(errs.CodeJupiterUnavailable, "test"))
	e.assertLeftCreated(t, errs.CodeJupiterUnavailable)
}

func TestSwapLayer_PrivyDownBeforeSubmit_LeavesCreated(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.privy.Fail("SignTransaction", errs.New(errs.CodePrivyUnavailable, "test"))
	e.assertLeftCreated(t, errs.CodePrivyUnavailable)
}

func (e *layerEnv) assertLeftCreated(t *testing.T, want errs.Code) {
	t.Helper()
	req := e.request(e.source())
	if _, err := e.run(t, req); errs.CodeOf(err) != want {
		t.Fatalf("Run = %v, want %s", err, want)
	}
	rows := e.swapIDs(t, req.Source.ID)
	if len(rows) != 1 {
		t.Fatalf("%d rows, want 1", len(rows))
	}
	status, signed, _, _ := e.row(t, rows[0])
	if status != "created" || len(signed) != 0 || len(e.events(t, rows[0])) != 0 {
		t.Fatalf("row = %s with %d signed bytes and events %v", status, len(signed), e.events(t, rows[0]))
	}
}

func TestSwapLayer_ForeignRowIsReturnedWithoutAnEvent(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	req := e.request(e.source())
	e.signer = signerFunc(
		func(ctx context.Context, wallet string, unsigned []byte) ([]byte, platform.Signature, error) {
			_, err := e.pool.Exec(ctx, `UPDATE swaps SET status = 'failed', failure_code = 'never_submitted',
			failed_at = now() WHERE source_id = $1`, req.Source.ID)
			if err != nil {
				return nil, "", err
			}
			return e.chainSigner().Sign(ctx, wallet, unsigned)
		},
	)
	got, err := e.run(t, req)
	if err != nil || got.Status != domain.StatusFailed {
		t.Fatalf("Run = %+v, %v, want the failed row another path wrote", got, err)
	}
	if events := e.events(t, got.ID.UUID()); len(events) != 0 {
		t.Fatalf("events = %v, want none from the loser", events)
	}
}

func TestSwapLayer_RunRefusesAmountsTheColumnsCannotHold(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	for name, mutate := range map[string]func(*app.SwapRequest){
		"in amount":        func(r *app.SwapRequest) { r.InAmount = 1 << 63 },
		"quote out amount": func(r *app.SwapRequest) { r.QuoteOutAmount = 1 << 63 },
		"negative slip":    func(r *app.SwapRequest) { r.SlippageBps = -1 },
		"huge slip":        func(r *app.SwapRequest) { r.SlippageBps = 1 << 31 },
	} {
		req := e.request(e.source())
		mutate(&req)
		if _, err := e.run(t, req); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("%s: Run = %v, want invalid_input", name, err)
		}
		if rows := e.swapIDs(t, req.Source.ID); len(rows) != 0 {
			t.Fatalf("%s: wrote %d rows", name, len(rows))
		}
	}
}

func TestSwapLayer_ClaimRefusedByTheTable(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	req := e.request(e.source())
	req.Action = "hold"
	if _, err := e.run(t, req); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Run = %v, want the check violation as internal", err)
	}
}

func TestSwapLayer_ClaimAndExistingClaimBothUnreachable(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	if _, err := e.pool.Exec(t.Context(), `ALTER TABLE swaps RENAME TO swaps_gone`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.run(t, e.request(e.source())); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Run = %v, want internal", err)
	}
}

func TestSwapLayer_ResultCannotBeRead(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.beforeSigning(func(ctx context.Context) error {
		_, err := e.pool.Exec(ctx, `ALTER VIEW swap_views RENAME TO swap_views_gone`)
		return err
	})
	if _, err := e.run(t, e.request(e.source())); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Run = %v, want internal", err)
	}
}

func TestSwapLayer_SubmitWriteRefusedForDuplicateSignedBytes(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	if _, err := e.run(t, e.request(e.source())); err != nil {
		t.Fatal(err)
	}
	if _, err := e.run(t, e.request(e.source())); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("a second swap with the same signed bytes = %v, want the unique violation", err)
	}
}

func TestSwapLayer_EventNeedsAnActor(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	if _, err := e.layer().Run(t.Context(), e.request(e.source()), nil); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Run without an actor = %v, want internal", err)
	}
}

func TestSwapLayer_HeartbeatTicksWhileTheSwapIsInFlight(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	var beats atomic.Int64
	e.signer = signerFunc(
		func(ctx context.Context, wallet string, unsigned []byte) ([]byte, platform.Signature, error) {
			testkit.Eventually(t, func() bool {
				e.clk.Advance(10 * time.Second)
				return beats.Load() > 0
			}, 5*time.Second)
			return e.chainSigner().Sign(ctx, wallet, unsigned)
		},
	)
	if _, err := e.layer().Run(actorContext(t.Context()), e.request(e.source()), func() { beats.Add(1) }); err != nil {
		t.Fatal(err)
	}
}
