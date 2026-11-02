package app

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const pastUint64 = "18446744073709551616"

func pastUnits(t *testing.T) money.Units {
	t.Helper()
	v, err := money.ParseUnits(pastUint64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func wantInvalid(t *testing.T, what string, err error) {
	t.Helper()
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("%s = %v, want invalid_input", what, err)
	}
}

type feePayer struct{}

func (feePayer) FeePayer() (chain.SolanaAddress, error) { return "payer", nil }

func (feePayer) Sign(context.Context, string, []byte) ([]byte, chain.Signature, error) {
	return nil, "", nil
}

func TestWideAmounts_aSolanaSwapRefusesAnAmountPastUint64WhereverItIsCarried(t *testing.T) {
	t.Parallel()
	past := pastUnits(t)
	req := SwapRequest{Action: domain.ActionBuy, InAmount: past}
	layer := &SwapLayer{d: SwapLayerDeps{Signer: feePayer{}}}

	_, orderErr := layer.order(t.Context(), req, "payer", 100)
	wantInvalid(t, "order", orderErr)

	handler := &ExecuteTradeHandler{d: ExecuteTradeDeps{Layer: layer}}
	_, priceErr := handler.price(t.Context(), ExecuteTrade{}, &req)
	wantInvalid(t, "price", priceErr)

	gen := testkit.NewIDs(1)
	winner, next := multichain.ListingIDFrom(gen.NewV7()), multichain.ListingIDFrom(gen.NewV7())
	cmd := ExecuteTrade{Action: domain.ActionBuy, Route: &multichain.Comparison{Ranked: []multichain.Quote{
		{ListingID: winner, Network: chain.NetworkSolana}, {ListingID: next, Network: chain.NetworkSolana},
	}}}
	_, _, fallbackErr := handler.fallback(
		t.Context(),
		cmd,
		req,
		multichain.Listing{ID: winner, Network: chain.NetworkSolana},
	)
	wantInvalid(t, "fallback", fallbackErr)
}

func TestWideAmounts_clampUint64KeepsWhatFitsAndSaturatesTheRest(t *testing.T) {
	t.Parallel()
	if got := clampUint64(money.UnitsFromUint64(7)); got != 7 {
		t.Fatalf("clampUint64(7) = %d", got)
	}
	if got := clampUint64(pastUnits(t)); got != math.MaxUint64 {
		t.Fatalf("clampUint64(2^64) = %d, want max uint64", got)
	}
}

func TestWideAmounts_aSolanaConfirmCannotBookUSDCPastMicros(t *testing.T) {
	t.Parallel()
	gen := testkit.NewIDs(2)
	buy := SwapRequest{Action: domain.ActionBuy, InAmount: pastUnits(t)}
	if _, err := confirmed(
		buy,
		gen.NewV7(),
		"sig",
		money.UnitsFromUint64(1),
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidInput {
		t.Fatalf("confirmed buy = %v, want invalid_input", err)
	}
	sell := SwapRequest{Action: domain.ActionSell, InAmount: money.UnitsFromUint64(1)}
	if _, err := confirmed(sell, gen.NewV7(), "sig", pastUnits(t)); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("confirmed sell = %v, want invalid_input", err)
	}
	layer := &SwapLayer{d: SwapLayerDeps{Chain: &sweeperReader{inbound: money.NewBaseUnits(5, 6)}}}
	err := layer.confirm(t.Context(), buy, gen.NewV7(), "sig")
	wantInvalid(t, "confirm", err)
}

func TestWideAmounts_aSweptSwapWhoseInAmountPassesMicrosIsNotConfirmed(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	row := sqlc.ListStaleSubmittedRow{
		ID: e.ids.NewV7(), CabalID: e.ids.NewV7(), SourceKind: "proposal", SourceID: e.ids.NewV7(),
		Action: "buy", Symbol: "AAPLx", InMint: "usdc", InAmount: pastUint64, SourceBatchSize: 1, OutDecimals: 8,
	}
	_, err := e.poller().resolveSubmitted(tickContext(t), e.clk.Now(), row, SigStatus{State: SigFinalized})
	wantInvalid(t, "resolveSubmitted", err)
}

func TestWideAmounts_aForcedSellPastMicrosIsRefusedAndTheSwapStaysSubmitted(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	e.landed()
	row := e.submit(t)
	if _, err := e.pool.Exec(t.Context(), `UPDATE swaps SET action = 'sell' WHERE id = $1`, row.ID); err != nil {
		t.Fatal(err)
	}
	cmd := forceCommand(chain.Signature("sig-"+row.ID.String()), domain.StatusConfirmed)
	cmd.OutAmount, cmd.Override = pastUnits(t), true

	_, err := e.force().Handle(t.Context(), cmd)

	wantInvalid(t, "Handle", err)
	if status := e.status(t, row.ID); status != "submitted" {
		t.Fatalf("status = %s, want submitted", status)
	}
}

func TestWideAmounts_aRoutedConfirmWithAFeePastInt64IsRefusedBeforeTheDatabase(t *testing.T) {
	t.Parallel()
	b := routedBuy{fee: money.MicrosFromUint64(math.MaxUint64)}
	fill := EVMSwapResult{EVMFill: EVMFill{State: FillSucceeded, Out: money.UnitsFromUint64(1)}}

	n, err := b.confirmed(money.MicrosFromUint64(1), fill).apply(t.Context(), nil, time.Time{})

	if n != 0 || errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("apply = %d, %v, want decode_failed", n, err)
	}
}

func TestWideAmounts_aCashOutLegsAreRefusedWhenTheirAmountsPassUint64(t *testing.T) {
	t.Parallel()
	h := &SellForCashOutHandler{}
	listing := &multichain.Listing{Network: chain.NetworkBase, TokenAddress: "0x1"}
	req := SwapRequest{QuoteOutAmount: pastUnits(t)}
	wantInvalid(t, "sell", h.sell(t.Context(), req, listing, nil))

	held := Holding{Mint: chain.Mint{Address: "aaplx", Decimals: 8}, Units: pastUnits(t)}
	_, err := h.value(t.Context(), ids.CabalID{}, held)
	wantInvalid(t, "value", err)
}

func TestWideAmounts_theCashLegIsCappedByWhatTheWalletHolds(t *testing.T) {
	t.Parallel()
	onChain := money.MicrosFromUint64(500)
	if got := capMicros(money.UnitsFromUint64(300), onChain); got.Uint64() != 300 {
		t.Fatalf("cap(300, 500) = %s, want 300", got)
	}
	if got := capMicros(money.UnitsFromUint64(900), onChain); got.Uint64() != 500 {
		t.Fatalf("cap(900, 500) = %s, want 500", got)
	}
	if got := capMicros(pastUnits(t), onChain); got.Uint64() != 500 {
		t.Fatalf("cap(2^64, 500) = %s, want 500", got)
	}
}
