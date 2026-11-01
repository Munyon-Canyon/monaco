package trading_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func TestRetry_aTradeBlockedBeforeAnySwapRunsAgainThroughTheNormalChecks(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	e.jup.Fail("Quote", errs.New(errs.CodeJupiterRejected, "jupiter.Quote"))
	cmd := e.buy()
	d := e.delivery(t, cmd)
	if err := e.handle(t, d, cmd); err != nil {
		t.Fatal(err)
	}
	if got := e.blockedCodes(t, cmd); got != "trade_not_started" || len(e.swapsOf(t, cmd.ProposalID)) != 0 {
		t.Fatalf("blocked %q with %d swaps, want trade_not_started and no swap", got, len(e.swapsOf(t, cmd.ProposalID)))
	}
	e.jup.Fail("Quote", nil)

	ev := busevents.TradeRetryRequested{
		V: 1, SwapID: uuid.Nil, CabalID: cmd.CabalID.UUID(),
		Source: busevents.TradeSource{Kind: "proposal", ID: cmd.ProposalID.UUID()}, Action: "buy", Symbol: cmd.Symbol,
		InMint: chain.SolanaAddress(usdcMint), OutMint: cmd.Mint, InAmount: buyMicros, QuoteOutAmount: quotedOut,
	}
	retry := e.delivery(t, cmd)
	retry.Handler = retryHandler
	if err := (adapters.Engine{Trades: e.handler()}).HandleRetry(actorContext(t.Context()), retry, ev); err != nil {
		t.Fatal(err)
	}
	if swaps := e.swapsOf(t, cmd.ProposalID); len(swaps) != 1 || e.deliveryCode(t, retry) == "trade_not_started" {
		t.Fatalf("%d swaps, delivery %q, want the retry to create the swap", len(swaps), e.deliveryCode(t, retry))
	}
}

func TestRetry_aTradeStillFailingItsChecksIsBlockedAgain(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	e.jup.Fail("Quote", errs.New(errs.CodeJupiterRejected, "jupiter.Quote"))
	cmd := e.buy()
	ev := busevents.TradeRetryRequested{
		V: 1, CabalID: cmd.CabalID.UUID(),
		Source: busevents.TradeSource{Kind: "proposal", ID: cmd.ProposalID.UUID()}, Action: "buy", Symbol: cmd.Symbol,
		InMint: chain.SolanaAddress(usdcMint), OutMint: cmd.Mint, InAmount: buyMicros, QuoteOutAmount: quotedOut,
	}
	retry := e.delivery(t, cmd)
	retry.Handler = retryHandler
	if err := (adapters.Engine{Trades: e.handler()}).HandleRetry(actorContext(t.Context()), retry, ev); err != nil {
		t.Fatal(err)
	}
	if got := e.blockedCodes(t, cmd); got != "trade_not_started" || len(e.swapsOf(t, cmd.ProposalID)) != 0 {
		t.Fatalf("blocked %q with %d swaps, want trade_not_started and no swap", got, len(e.swapsOf(t, cmd.ProposalID)))
	}
}
