package trading_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	swapHash   = "0xhash2"
	baseCash   = "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913"
	baseToken  = "0x00000000000000000000000000000000000000bb"
	baseRouter = "0x00000000000000000000000000000000000000cc"
)

type evmSwapEnv struct {
	stepEnv
	clk *testkit.Clock
	evm *evmFake
	leg app.EVMSwapLeg
}

func evmAddress(t *testing.T, raw string) chain.EVMAddress {
	t.Helper()
	a, err := chain.ParseEVMAddress(raw)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func newEVMSwapEnv(t *testing.T) evmSwapEnv {
	t.Helper()
	d := newSwapDB(t)
	d.now = clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	row := d.created(d.ids.NewV7(), "base:"+baseCash)
	row.OutMint, row.TreasuryAddress, row.InAmount = "base:"+baseToken, evmTreasury, 10_000_000
	d.insert(t, row)
	e := evmSwapEnv{
		stepEnv: stepEnv{swapDB: d, ref: domain.StepRef{
			SwapID: ids.SwapIDFrom(row.ID), CabalID: ids.CabalIDFrom(row.CabalID),
			Step: domain.StepSwap, Network: chain.NetworkBase,
		}},
		clk: testkit.NewClock(d.now),
	}
	e.journal = e.journalOn(steppingClock{e.clk})
	tx := func(to string) multichain.EVMTransaction {
		return multichain.EVMTransaction{Network: chain.NetworkBase, To: evmAddress(t, to), Data: []byte{1}}
	}
	e.evm = newEVMFake(app.EVMSwapPlan{
		Approvals: []multichain.EVMTransaction{tx(baseCash)}, Swap: tx(baseRouter), MinOut: 300,
	})
	e.leg = app.EVMSwapLeg{
		Request: app.SwapRequest{
			Source:  domain.Source{Kind: domain.SourceProposal, ID: row.SourceID},
			CabalID: e.ref.CabalID, Action: domain.ActionBuy, Symbol: row.Symbol,
			InMint:   chain.Mint{Address: chain.SolanaAddress(row.InMint), Decimals: 6},
			OutMint:  chain.Mint{Address: chain.SolanaAddress(row.OutMint), Decimals: 8},
			InAmount: 10_000_000, SourceBatchSize: 1,
		},
		Ref: e.ref, WalletID: "evm-wallet", Amount: money.MicrosFromUint64(9_992_500),
		Spec: app.EVMSwapSpec{
			Network: chain.NetworkBase, Owner: evmAddress(t, evmTreasury), TokenIn: evmAddress(t, baseCash),
			TokenOut: evmAddress(t, baseToken), AmountIn: 9_992_500, MinOut: 300,
		},
	}
	return e
}

func (e evmSwapEnv) run(ctx context.Context) (app.EVMSwapResult, error) {
	return app.EVMSwap{Journal: e.journal, Venue: e.evm, Sender: e.evm, Poll: app.DefaultPoll()}.
		Run(actorContext(ctx), e.leg)
}

func (e evmSwapEnv) key() string { return "trade:" + e.ref.SwapID.String() + ":swap" }

func (e evmSwapEnv) swapRow(t *testing.T) (status, signature string) {
	t.Helper()
	var sig *string
	if err := e.pool.QueryRow(t.Context(), `SELECT status, tx_signature FROM swaps WHERE id = $1`,
		e.ref.SwapID.UUID()).Scan(&status, &sig); err != nil {
		t.Fatal(err)
	}
	if sig != nil {
		signature = *sig
	}
	return status, signature
}

func (e evmSwapEnv) submittedEvents(t *testing.T) []string {
	t.Helper()
	rows, err := e.pool.Query(t.Context(), `SELECT payload->>'tx_signature' FROM events
		WHERE type = 'trade.submitted' AND aggregate_id = $1 ORDER BY id`, e.ref.SwapID.UUID())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var sig string
		if err := rows.Scan(&sig); err != nil {
			t.Fatal(err)
		}
		out = append(out, sig)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (e evmSwapEnv) wantUnsent(t *testing.T) {
	t.Helper()
	if s := e.step(t); s.Status != domain.StepPending || s.PrivyActionID != "" {
		t.Fatalf("step = %+v, want pending with no hash", s)
	}
	if status, sig := e.swapRow(t); status != string(domain.StatusCreated) || sig != "" {
		t.Fatalf("swap row %s %q, want created with no hash", status, sig)
	}
	if n := len(e.submittedEvents(t)); n != 0 {
		t.Fatalf("%d trade.submitted events, want none", n)
	}
}

func (e evmSwapEnv) wantSubmittedOnce(t *testing.T) {
	t.Helper()
	hash := swapHash
	if s := e.step(t); s.Status != domain.StepSubmitted || s.PrivyActionID != hash || s.Amount != e.leg.Amount {
		t.Fatalf("step = %+v, want submitted with %s", s, hash)
	}
	if status, sig := e.swapRow(t); status != string(domain.StatusSubmitted) || sig != hash {
		t.Fatalf("swap row %s %q, want submitted with %s", status, sig, hash)
	}
	if got := e.submittedEvents(t); !slices.Equal(got, []string{hash}) {
		t.Fatalf("trade.submitted signatures = %v, want [%s]", got, hash)
	}
}

func TestEVMSwap_sendsTheApprovalsThenTheSwapStoresTheHashAndReadsTheFill(t *testing.T) {
	t.Parallel()
	e := newEVMSwapEnv(t)
	e.evm.fills = []app.EVMFill{{State: app.FillPending}, {State: app.FillSucceeded, Out: 306}}

	got, err := e.run(t.Context())
	want := app.EVMSwapResult{EVMFill: app.EVMFill{State: app.FillSucceeded, Out: 306}, Hash: swapHash}
	if err != nil || got != want {
		t.Fatalf("Run = %+v, %v, want %+v", got, err, want)
	}
	if keys := e.evm.keys(); !slices.Equal(keys, []string{e.key() + ":approve:0", e.key()}) {
		t.Fatalf("sent %v, want the approval then the swap", keys)
	}
	e.wantSubmittedOnce(t)

	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: 306}}
	if again, err := e.run(t.Context()); err != nil || again != want {
		t.Fatalf("resumed Run = %+v, %v, want the same fill", again, err)
	}
	if len(e.evm.keys()) != 2 || e.evm.builds != 1 {
		t.Fatalf("the resume sent %v after %d builds, want nothing new", e.evm.keys(), e.evm.builds)
	}
}

func TestEVMSwap_aCrashAfterTheSendResumesUnderTheSameKeyWithOneSwap(t *testing.T) {
	t.Parallel()
	e := newEVMSwapEnv(t)
	ctx, crash := context.WithCancel(t.Context())
	e.evm.afterSend[e.key()] = crash
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}

	if _, err := e.run(ctx); err == nil {
		t.Fatal("the crashed run returned no error")
	}
	e.wantUnsent(t)
	got, err := e.run(t.Context())
	if err != nil || got.State != app.FillReverted || got.Hash != swapHash {
		t.Fatalf("resumed Run = %+v, %v, want the reverted fill of 0xhash2", got, err)
	}

	if e.evm.hashes() != 2 || !slices.Equal(e.evm.keys(), []string{e.key() + ":approve:0", e.key(), e.key()}) {
		t.Fatalf("sent %v as %d distinct transactions, want the swap sent twice as one after one approval",
			e.evm.keys(), e.evm.hashes())
	}
	e.wantSubmittedOnce(t)
}

func TestEVMSwap_aResumeSendsTheStoredCalldataNotWhatTheVenueBuildsNow(t *testing.T) {
	t.Parallel()
	e := newEVMSwapEnv(t)
	ctx, crash := context.WithCancel(t.Context())
	e.evm.afterSend[e.key()] = crash
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: 306}}

	if _, err := e.run(ctx); err == nil {
		t.Fatal("the crashed run returned no error")
	}
	e.evm.plan.Swap.Data = []byte{9, 9}
	e.evm.plan.MinOut = 1
	if got, err := e.run(t.Context()); err != nil || got.State != app.FillSucceeded {
		t.Fatalf("resumed Run = %+v, %v, want the swap read to its fill", got, err)
	}

	sent := e.evm.sentTxs[e.key()]
	if len(sent) != 2 || !slices.Equal(sent[0].Data, []byte{1}) || !slices.Equal(sent[1].Data, sent[0].Data) {
		t.Fatalf("swap sent as %+v, want the first calldata both times under one key", sent)
	}
	if e.evm.builds != 1 {
		t.Fatalf("%d builds, want the resume to rebuild nothing", e.evm.builds)
	}
}

func TestEVMSwap_aFillReadErrorLeavesTheSubmittedSwapForTheResume(t *testing.T) {
	t.Parallel()
	e := newEVMSwapEnv(t)
	e.evm.fillErr = errs.New(errs.CodeRPCUnavailable, "evm.Fill")

	if _, err := e.run(t.Context()); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("Run err = %v, want rpc_unavailable", err)
	}
	e.wantSubmittedOnce(t)
}

func TestEVMSwap_aFillStillPendingAfterTheBudgetReturnsPending(t *testing.T) {
	t.Parallel()
	e := newEVMSwapEnv(t)

	got, err := e.run(t.Context())
	if err != nil || got.State != app.FillPending || got.Hash != swapHash {
		t.Fatalf("Run = %+v, %v, want a pending fill of 0xhash2", got, err)
	}
	e.wantSubmittedOnce(t)
}

func TestEVMSwap_aPlanUnderTheFloorSendsNothing(t *testing.T) {
	t.Parallel()
	e := newEVMSwapEnv(t)
	e.evm.plan.MinOut = 299

	if _, err := e.run(t.Context()); errs.CodeOf(err) != errs.CodePriceMoved {
		t.Fatalf("Run err = %v, want price_moved", err)
	}
	if len(e.evm.keys()) != 0 {
		t.Fatalf("sent %v under the floor", e.evm.keys())
	}
	e.wantUnsent(t)
}

func TestEVMSwap_aBuildOrSendErrorLeavesTheStepPending(t *testing.T) {
	t.Parallel()
	cases := map[string]func(e evmSwapEnv){
		"build":    func(e evmSwapEnv) { e.evm.buildErr = errs.New(errs.CodeUpstreamUnavailable, "evm.Build") },
		"approval": func(e evmSwapEnv) { e.evm.sendErr[e.key()+":approve:0"] = errs.New(errs.CodePrivyUnavailable, "") },
		"swap":     func(e evmSwapEnv) { e.evm.sendErr[e.key()] = errs.New(errs.CodePrivyUnavailable, "") },
	}
	for name, fail := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEVMSwapEnv(t)
			fail(e)
			if _, err := e.run(t.Context()); err == nil {
				t.Fatal("Run returned no error")
			}
			e.wantUnsent(t)
		})
	}
}

func TestEVMSwap_aSwapRowNoLongerCreatedRollsTheSubmitBack(t *testing.T) {
	t.Parallel()
	e := newEVMSwapEnv(t)
	e.fail(t, e.ref.SwapID.UUID(), string(domain.FailureNeverSubmitted))

	if _, err := e.run(t.Context()); errs.CodeOf(err) != errs.CodeVersionConflict {
		t.Fatalf("Run err = %v, want version_conflict", err)
	}
	if s := e.step(t); s.Status != domain.StepPending {
		t.Fatalf("step = %+v, want pending after the rollback", s)
	}
	if n := len(e.submittedEvents(t)); n != 0 {
		t.Fatalf("%d trade.submitted events, want none", n)
	}
}

func TestEVMSwap_aZeroAmountIsRefusedBeforeAnySend(t *testing.T) {
	t.Parallel()
	e := newEVMSwapEnv(t)
	e.leg.Amount = money.Micros{}

	if _, err := e.run(t.Context()); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Run err = %v, want invalid_input", err)
	}
	if e.evm.builds != 0 {
		t.Fatal("a refused leg built a swap")
	}
}

func TestEVMSwap_aSendWithoutAHashIsRefused(t *testing.T) {
	t.Parallel()
	e := newEVMSwapEnv(t)
	e.evm.byKey[e.key()] = ""

	if _, err := e.run(t.Context()); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Run err = %v, want invalid_input", err)
	}
	e.wantUnsent(t)
}

func TestEVMSwap_aRunThatFindsTheHashAlreadyStoredReadsTheFill(t *testing.T) {
	t.Parallel()
	e := newEVMSwapEnv(t)
	e.evm.afterSend[e.key()] = func() {
		ctx := actorContext(t.Context())
		if _, err := e.journal.Submitted(ctx, e.ref, swapHash); err != nil {
			t.Error(err)
		}
		if _, err := e.q.MarkSubmittedEVM(ctx, sqlc.MarkSubmittedEVMParams{
			ID: e.ref.SwapID.UUID(), TxHash: swapHash, SubmittedAt: e.now,
		}); err != nil {
			t.Error(err)
		}
	}
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: 306}}

	got, err := e.run(t.Context())
	if err != nil || got.State != app.FillSucceeded || got.Hash != swapHash {
		t.Fatalf("Run = %+v, %v, want the fill of the stored hash", got, err)
	}
	if n := len(e.submittedEvents(t)); n != 0 {
		t.Fatalf("%d trade.submitted events, want none from the run that lost the race", n)
	}
}

func TestEVMSwap_sendsEveryTransactionThroughPrivyUnderItsStepKey(t *testing.T) {
	t.Parallel()
	e := newEVMSwapEnv(t)
	wallets := &recordingWallets{}
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: 306}}

	got, err := app.EVMSwap{
		Journal: e.journal, Venue: e.evm, Sender: adapters.EVMSend{Wallets: wallets}, Poll: app.DefaultPoll(),
	}.Run(actorContext(t.Context()), e.leg)

	if err != nil || got.State != app.FillSucceeded || got.Hash != "0xsent2" {
		t.Fatalf("Run = %+v, %v, want the fill of the swap Privy sent", got, err)
	}
	if keys := wallets.keys(); !slices.Equal(keys, []string{e.key() + ":approve:0", e.key()}) {
		t.Fatalf("Privy got keys %v, want the approval then the swap under trade:<swap>:swap", keys)
	}
	for _, s := range wallets.sent {
		if s.wallet != "evm-wallet" || !s.sponsor {
			t.Fatalf("sent %+v, want a sponsored send from the EVM treasury wallet", s)
		}
	}
}

type venueFills struct {
	*adapters.EVMVenue
	fake *evmFake
}

func (v venueFills) Fill(
	ctx context.Context,
	n chain.Network,
	hash string,
	out, owner chain.EVMAddress,
) (app.EVMFill, error) {
	return v.fake.Fill(ctx, n, hash, out, owner)
}

func (v venueFills) Receipt(ctx context.Context, n chain.Network, hash string) (app.EVMFillState, error) {
	return v.fake.Receipt(ctx, n, hash)
}

func TestEVMSwap_aBuyWhoseExecutionQuoteBeatsTheRequoteStillGoesThrough(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, probeOut, minAmount string
	}{
		{"unchanged", "1000000", "990100"},
		{"improved_5_bps", "1000500", "990196"},
		{"improved_50_bps", "1005000", "990126"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newEVMSwapEnv(t)
			v := newVenueEnv(t)
			approveData := abiCall("095ea7b3", word(permit2), word("9992500"))
			v.script(
				approvalReply(evmTxJSON(baseCash, approveData, "0x00")),
				quoteReply(c.probeOut, c.probeOut, "null"),
				quoteReply(c.probeOut, c.minAmount, "null"),
				swapReply(evmTxJSON(venueRouter, "0x3593564c", "0x0")),
			)
			e.leg.Spec = v.spec(t)
			e.leg.Spec.SlippageBps = 100
			e.leg.Spec.Owner = evmAddress(t, evmTreasury)
			e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: 990_000}}

			got, err := app.EVMSwap{
				Journal: e.journal,
				Venue:   venueFills{EVMVenue: v.venue, fake: e.evm},
				Sender:  e.evm,
				Poll:    app.DefaultPoll(),
			}.Run(actorContext(t.Context()), e.leg)

			if err != nil || got.State != app.FillSucceeded {
				t.Fatalf("Run = %+v, %v, want the buy to go through", got, err)
			}
		})
	}
}
