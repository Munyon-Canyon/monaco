package trading_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (e *layerEnv) sell(outAmount uint64) app.SwapRequest {
	e.jup.SetOrder(jupiterMint(aaplxToken()), jupiterMint(usdcToken()), jupiter.Order{
		RequestID: "req-sell", Transaction: swapTx(),
	})
	e.jup.SetExecute("req-sell", jupiter.ExecuteResult{Status: jupiter.StatusSuccess, OutAmount: outAmount})
	req := e.request(e.source())
	req.Action, req.InMint, req.OutMint, req.InAmount = domain.ActionSell, aaplxToken(), usdcToken(), 10_000_000
	req.QuoteOutAmount = outAmount
	return req
}

func TestSwapLayer_SellFillIsTheTreasurysChainDeltaNotUltrasPreFeeOutput(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.fills.charge(10)
	got := e.settled(t, e.sell(4_885_867))
	if got.Status != domain.StatusConfirmed || got.OutAmount != 4_880_982 {
		t.Fatalf("view = %+v, want confirmed at the chain's 4880982, not Ultra's 4885867", got)
	}
	p := e.payload(t, got.ID.UUID(), "trade.confirmed")
	if p["out_amount"] != "4880982" || p["usdc_micros"] != "4880982" {
		t.Fatalf("trade.confirmed payload = %v, want the chain delta", p)
	}
	if reads := e.fills.mints(); len(reads) != 1 || reads[0] != usdcToken() {
		t.Fatalf("chain reads = %+v, want one read of the treasury's USDC", reads)
	}
}

func TestSwapLayer_BuyFillMatchesTheChainDelta(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusSuccess, OutAmount: 105_000_000})
	req := e.request(e.source())
	got := e.settled(t, req)
	delta, err := e.fills.InboundAmount(t.Context(), got.TxSignature, req.TreasuryWallet.Address, req.OutMint)
	if err != nil || got.Status != domain.StatusConfirmed || got.OutAmount != 105_000_000 ||
		delta.Uint64() != got.OutAmount {
		t.Fatalf("view = %+v, chain delta %s (%v); want both 105000000", got, delta, err)
	}
}

func TestSwapLayer_SellWithAnUnreadableDeltaStaysSubmittedAndAlerts(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.fills.fail(errs.New(errs.CodeRPCUnavailable, "test"))
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(actorContext(t.Context()),
		observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	got, err := e.layer().Run(ctx, e.sell(4_885_867), nil)
	if err != nil || got.Status != domain.StatusSubmitted || got.OutAmount != 0 {
		t.Fatalf("Run = %+v, %v; want the sell left submitted with no fill", got, err)
	}
	if events := e.events(t, got.ID.UUID()); !slices.Equal(events, []string{"trade.submitted"}) {
		t.Fatalf("events = %v, want no trade.confirmed", events)
	}
	alerts := logs.Lines(observability.TradingSwapFillUnread.Name, "level", "swap_id", "code")
	want := "ERROR " + got.ID.String() + " rpc_unavailable"
	if len(alerts) != 1 || alerts[0] != want {
		t.Fatalf("alerts = %q, want one %q", alerts, want)
	}
}
