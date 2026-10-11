package treasury_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func newBridgedSaleRig(t *testing.T) *saleRig {
	t.Helper()
	f := newFixture(t)
	r := &saleRig{f: f, alice: f.user(t), bob: f.user(t), cabal: f.cabal(t)}
	cashOutFund(t, f, r.alice, r.cabal, 50_000_000)
	cashOutFund(t, f, r.bob, r.cabal, 50_000_000)
	if err := f.confirm(t, buy(r.cabal, f.ids.NewV7(), 40_000_000, 1_000, 0)); err != nil {
		t.Fatal(err)
	}
	f.bridgedOut(t, r.cabal, 40_000_000, 30_000)
	if err := f.postCabal(evmBuy(t, f, r.cabal, 39_970_000, 400_000_000)); err != nil {
		t.Fatal(err)
	}
	h := cashOutHandlerWith(f, cashOutPauses{}, cashOutValues{pot: money.MicrosFromUint64(100_000_000)})
	ctx := observability.WithActor(f.ctx(), "user:"+r.alice.String())
	result, err := h.Handle(ctx, app.CashOut{CabalID: r.cabal, UserID: r.alice, All: true})
	if err != nil {
		t.Fatal(err)
	}
	r.job = result.ID
	var sell string
	if err := f.pool.QueryRow(t.Context(), `SELECT sell_usdc_micros::text FROM cash_out_jobs WHERE id = $1`, r.job).
		Scan(&sell); err != nil || sell != "30000000" {
		t.Fatalf("job sells %s, %v, want the 30 USDC the Solana cash does not cover", sell, err)
	}
	r.started = events.CashOutStarted{
		V: 1, JobID: r.job, CabalID: r.cabal.UUID(), UserID: r.alice.UUID(), ShareUnits: 100,
		PayoutMicros: money.MicrosFromUint64(50_000_000), SellUSDC: money.MicrosFromUint64(30_000_000),
	}
	return r
}

func (r *saleRig) evmSold(swap uuid.UUID, units, micros uint64) events.TradeConfirmed {
	return events.TradeConfirmed{
		V: 1, SwapID: swap, CabalID: r.cabal.UUID(), Source: events.TradeSource{Kind: "cashout", ID: r.job},
		SourceBatchSize: 2, Action: "sell", Symbol: "AAPLx", InMint: baseAAPL, InAmount: units,
		OutMint: baseCash, OutAmount: micros, USDCMicros: money.MicrosFromUint64(micros),
		TxSignature: "0xsell", ConfirmedAt: confirmedAt(),
	}
}

func (r *saleRig) back(swap uuid.UUID, status string, sent, fee uint64, batch int) events.TradeStepChanged {
	return events.TradeStepChanged{
		V: 1, SwapID: swap, CabalID: r.cabal.UUID(), Source: events.TradeSource{Kind: "cashout", ID: r.job},
		SourceBatchSize: batch, Step: "bridge_back", Network: chain.NetworkBase, Status: status,
		PrivyActionID: "privy-back-" + swap.String(), AmountMicros: money.MicrosFromUint64(sent),
		FeeMicros: money.MicrosFromUint64(fee),
	}
}

func TestCashOutSale_sellsTheSolanaAndTheEVMLotAndPaysOnceTheEVMProceedsLand(t *testing.T) {
	t.Parallel()
	r := newBridgedSaleRig(t)
	r.deliver(t, r.started)
	r.deliver(t, r.sold(r.f.ids.NewV7(), 500, 15_000_000, 2))
	evm := r.f.ids.NewV7()

	r.deliver(t, r.evmSold(evm, 150_000_000, 15_100_000))
	r.want(t, "selling", "50000000", "0", "", 0)
	r.deliver(t, r.back(evm, "pending", 15_100_000, 0, 2))
	r.deliver(t, r.back(evm, "submitted", 15_100_000, 0, 2))
	r.want(t, "selling", "50000000", "0", "", 0)
	r.deliver(t, r.back(evm, "succeeded", 15_100_000, 100_000, 2))

	r.want(t, "paying", "50000000", "0", "", 0)
}

func TestCashOutSale_anEVMLotWhoseProceedsFailToComeBackSettlesShort(t *testing.T) {
	t.Parallel()
	r := newBridgedSaleRig(t)
	r.deliver(t, r.started)
	r.deliver(t, r.sold(r.f.ids.NewV7(), 500, 15_000_000, 2))
	evm := r.f.ids.NewV7()
	r.deliver(t, r.evmSold(evm, 150_000_000, 15_100_000))
	r.deliver(t, r.back(evm, "submitted", 15_100_000, 0, 2))

	r.deliver(t, r.back(evm, "failed", 15_100_000, 0, 2))

	r.want(t, "paying", "35000000", "30", "sale_short", 30)
}

func TestCashOutSale_refusesALandedBridgeBackWhoseFeeExceedsItsAmount(t *testing.T) {
	t.Parallel()
	r := newBridgedSaleRig(t)
	h := adapters.CashOut{Sales: app.NewCashOutSales(r.f.ledger, r.f.ids)}
	swap := r.back(r.f.ids.NewV7(), "succeeded", 10, 11, 1)
	swap.Step = "swap"
	bad := r.back(r.f.ids.NewV7(), "succeeded", 10, 11, 1)

	err := r.f.do(func(ctx context.Context, tx db.Tx) error {
		if err := h.BridgedBack(ctx, tx, swap, confirmedAt()); err != nil {
			return err
		}
		return h.BridgedBack(ctx, tx, bad, confirmedAt())
	})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("BridgedBack err = %v, want invalid_input", err)
	}
}

func TestCashOutSale_paysTheSolanaAmountLandedNotTheEVMProceeds(t *testing.T) {
	t.Parallel()
	r := newBridgedSaleRig(t)
	r.deliver(t, r.started)
	r.deliver(t, r.sold(r.f.ids.NewV7(), 500, 15_000_000, 2))
	evm := r.f.ids.NewV7()
	r.deliver(t, r.evmSold(evm, 100_000_000, 10_100_000))
	r.deliver(t, r.back(evm, "submitted", 10_100_000, 0, 2))

	r.deliver(t, r.back(evm, "succeeded", 10_100_000, 100_000, 2))

	r.want(t, "paying", "45000000", "10", "sale_short", 10)
}

func TestTradeStepChanged_aPayloadWithoutTheSourceDecodesAndSettlesNothing(t *testing.T) {
	t.Parallel()
	r := newBridgedSaleRig(t)
	r.deliver(t, r.started)
	old := []byte(`{"v":1,"swap_id":"` + r.f.ids.NewV7().String() + `","cabal_id":"` + r.cabal.String() +
		`","step":"bridge_back","network":"base","status":"pending","privy_action_id":"",` +
		`"amount_micros":"15000000","fee_micros":"0"}`)
	decoded, err := events.Decode(events.TypeTradeStepChanged, 1, old)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := decoded.(events.TradeStepChanged)
	if !ok || e.Source != (events.TradeSource{}) || e.SourceBatchSize != 0 {
		t.Fatalf("decoded %#v, want zero source fields", decoded)
	}
	r.deliver(t, e)
	r.want(t, "selling", "50000000", "0", "", 0)
}

func TestCashOutSale_aShortSaleAcrossChainsPaysWhatBothLegsRaisedAndReturnsTheRest(t *testing.T) {
	t.Parallel()
	r := newBridgedSaleRig(t)
	r.deliver(t, r.started)
	r.deliver(t, r.sold(r.f.ids.NewV7(), 1_000, 12_000_000, 2))
	evm := r.f.ids.NewV7()
	r.deliver(t, r.evmSold(evm, 400_000_000, 10_100_000))
	r.deliver(t, r.back(evm, "submitted", 10_100_000, 0, 2))

	r.deliver(t, r.back(evm, "succeeded", 10_100_000, 100_000, 2))

	r.want(t, "paying", "42000000", "16", "sale_short", 16)
}

func TestCashOutSale_aRetriedBridgeBackThatLandsDoesNotSettleTheSaleAgain(t *testing.T) {
	t.Parallel()
	r := newBridgedSaleRig(t)
	r.deliver(t, r.started)
	evm := r.f.ids.NewV7()
	r.deliver(t, r.evmSold(evm, 150_000_000, 15_100_000))
	r.deliver(t, r.back(evm, "submitted", 15_100_000, 0, 2))
	r.deliver(t, r.back(evm, "failed", 15_100_000, 0, 2))
	for _, status := range []string{"pending", "submitted", "succeeded"} {
		retried := r.back(evm, status, 15_100_000, 100_000, 2)
		retried.Attempt = 2
		r.deliver(t, retried)
	}
	r.want(t, "selling", "50000000", "0", "", 0)

	r.deliver(t, r.sold(r.f.ids.NewV7(), 500, 15_000_000, 2))

	r.want(t, "paying", "35000000", "30", "sale_short", 30)
	h := adapters.CashOut{Sales: app.NewCashOutSales(r.f.ledger, r.f.ids)}
	bad := r.back(evm, "succeeded", 10, 11, 2)
	bad.Attempt = 2
	if err := r.f.do(func(ctx context.Context, tx db.Tx) error {
		return h.BridgedBack(ctx, tx, bad, confirmedAt())
	}); err != nil {
		t.Fatalf("BridgedBack of attempt 2 = %v, want it ignored before any sale result", err)
	}
}
