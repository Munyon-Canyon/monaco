package trading_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chaos"
)

const retryHandler = "trading.engine.retry"

func (b *engineBus) failFirstSwap(t *testing.T, cmd app.ExecuteTrade, in, out platform.Mint) uuid.UUID {
	t.Helper()
	b.jup.SetOrder(jupiterMint(in), jupiterMint(out), jupiter.Order{
		RequestID: "req-1", Transaction: chainfake.Unsigned(chainfake.WalletAddress(treasuryWallet)),
	})
	b.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusFailed, ErrorCode: 6001})
	b.dispatch(t, b.message(t, cmd, ""))
	next := chainfake.Unsigned(chainfake.WalletAddress(treasuryWallet))
	next[len(next)-2] = 1
	b.jup.SetOrder(jupiterMint(in), jupiterMint(out), jupiter.Order{RequestID: "req-2", Transaction: next})
	swaps := b.swapsOf(t, cmd.ProposalID)
	if got := b.statuses(t, cmd); len(got) != 1 || got[0] != "failed:jupiter_failed" {
		t.Fatalf("first swap %v, want one jupiter_failed swap", got)
	}
	return swaps[0]
}

func retryOf(swap uuid.UUID, cmd app.ExecuteTrade, slippageBps int32) busevents.TradeRetryRequested {
	ev := busevents.TradeRetryRequested{
		V: 1, SwapID: swap, CabalID: cmd.CabalID.UUID(),
		Source: busevents.TradeSource{Kind: string(domain.SourceProposal), ID: cmd.ProposalID.UUID()},
		Action: string(cmd.Action), Symbol: cmd.Symbol, InMint: usdcMint, OutMint: cmd.Mint,
		InAmount: cmd.USDCMicros.Uint64(), QuoteOutAmount: cmd.QuoteOutAmount, SlippageBps: slippageBps,
		RequestedBy: uuid.UUID{1},
	}
	if cmd.Action == domain.ActionSell {
		ev.InMint, ev.OutMint, ev.InAmount = cmd.Mint, usdcMint, cmd.TokenAmount
	}
	return ev
}

func (b *engineBus) retryMessage(t *testing.T, ev busevents.TradeRetryRequested) *engineMsg {
	t.Helper()
	var id uuid.UUID
	var payload json.RawMessage
	err := b.uow.Do(actorContext(t.Context()), func(ctx context.Context, tx db.Tx) error {
		return tx.Events.Append(ctx, ev)
	})
	if err == nil {
		err = b.pool.QueryRow(t.Context(), `SELECT id, payload FROM events WHERE type = 'trade.retry_requested'
			AND aggregate_id = $1 ORDER BY id DESC LIMIT 1`, ev.SwapID).Scan(&id, &payload)
	}
	if err != nil {
		t.Fatal(err)
	}
	eventID := ids.EventIDFrom(id)
	return &engineMsg{
		Msg: chaos.NewMsg(b.conn, busevents.TypeTradeRetryRequested, eventID, payload),
		id:  eventID, payload: payload, delivered: 1,
	}
}

func TestTradeEngine_RetryOfAFailedSwapConfirmsASecondSwap(t *testing.T) {
	t.Parallel()
	b := newEngineBus(t)
	cmd := b.buy()
	failed := b.failFirstSwap(t, cmd, usdcToken(), aaplxToken())
	msg := b.retryMessage(t, retryOf(failed, cmd, 100))

	b.dispatch(t, msg)
	again := &engineMsg{
		Msg: chaos.NewMsg(b.conn, busevents.TypeTradeRetryRequested, msg.id, msg.payload),
		id:  msg.id, payload: msg.payload, delivered: 2,
	}
	b.dispatch(t, again)
	got := b.statuses(t, cmd)
	recorded := b.count(t, `SELECT count(*) FROM event_deliveries WHERE handler = $1 AND event_id = $2`,
		retryHandler, msg.id.UUID())
	if msg.verdict != "ack" || again.verdict != "ack" || len(got) != 2 || got[1] != "confirmed" || recorded != 1 {
		t.Fatalf("verdicts %q, %q, swaps %v, %d delivery rows; want two acks, one confirmed second swap, one row",
			msg.verdict, again.verdict, got, recorded)
	}
}

func TestTradeEngine_TwoRetriesOfOneFailedSwapMakeOneNewSwap(t *testing.T) {
	t.Parallel()
	b := newEngineBus(t)
	cmd := b.buy()
	failed := b.failFirstSwap(t, cmd, usdcToken(), aaplxToken())
	first := b.retryMessage(t, retryOf(failed, cmd, 100))
	second := b.retryMessage(t, retryOf(failed, cmd, 100))

	b.dispatch(t, first)
	b.dispatch(t, second)
	got := b.statuses(t, cmd)
	if first.verdict != "ack" || second.verdict != "ack" || len(got) != 2 || len(b.jup.Sent("req-2")) != 1 {
		t.Fatalf("verdicts %q, %q, swaps %v, %d executes; want two acks and one new swap",
			first.verdict, second.verdict, got, len(b.jup.Sent("req-2")))
	}
}

func TestTradeEngine_RetryOfASupersededFailedSwapAcksWithoutASwap(t *testing.T) {
	t.Parallel()
	b := newEngineBus(t)
	cmd := b.buy()
	failed := b.failFirstSwap(t, cmd, usdcToken(), aaplxToken())
	b.jup.SetExecute("req-2", jupiter.ExecuteResult{Status: jupiter.StatusFailed, ErrorCode: 6001})
	b.dispatch(t, b.retryMessage(t, retryOf(failed, cmd, 100)))
	stale := b.retryMessage(t, retryOf(failed, cmd, 100))

	b.dispatch(t, stale)
	if got := b.statuses(t, cmd); stale.verdict != "ack" || len(got) != 2 {
		t.Fatalf("verdict %q, swaps %v; want an ack and no third swap", stale.verdict, got)
	}
}

func TestTradeEngine_RetryUsesThePayloadsSlippage(t *testing.T) {
	t.Parallel()
	b := newEngineBus(t)
	cmd := b.buy()
	failed := b.failFirstSwap(t, cmd, usdcToken(), aaplxToken())
	b.quote(usdcToken(), aaplxToken(), 20_370_000, true)
	msg := b.retryMessage(t, retryOf(failed, cmd, 300))

	b.dispatch(t, msg)
	if got := b.statuses(t, cmd); msg.verdict != "ack" || len(got) != 2 || got[1] != "confirmed" {
		t.Fatalf("verdict %q, swaps %v; want the 300 bps payload to admit a quote 100 bps would refuse",
			msg.verdict, got)
	}
}

func TestTradeEngine_RefusedRetryBlocksTheProposal(t *testing.T) {
	t.Parallel()
	b := newEngineBus(t)
	cmd := b.buy()
	failed := b.failFirstSwap(t, cmd, usdcToken(), aaplxToken())
	b.ledger.SetTokens(b.wallet.Address, usdcToken(), 1)
	msg := b.retryMessage(t, retryOf(failed, cmd, 100))

	b.dispatch(t, msg)
	blocked := b.blocked(t, cmd.ProposalID)
	if msg.verdict != "ack" || len(blocked) != 1 || blocked[0]["code"] != "insufficient_funds" ||
		len(b.statuses(t, cmd)) != 1 {
		t.Fatalf("verdict %q, blocked %v; want one insufficient_funds trade.blocked and no new swap",
			msg.verdict, blocked)
	}
}

func TestTradeEngine_RetryOfASellSpendsThePayloadsTokens(t *testing.T) {
	t.Parallel()
	b := newEngineBus(t)
	b.ledger.SetTokens(b.wallet.Address, aaplxToken(), 50_000_000)
	b.quote(aaplxToken(), usdcToken(), quotedOut, true)
	cmd := b.sell(50_000_000)
	failed := b.failFirstSwap(t, cmd, aaplxToken(), usdcToken())
	msg := b.retryMessage(t, retryOf(failed, cmd, 100))

	b.dispatch(t, msg)
	var in int64
	var inMint string
	if err := b.pool.QueryRow(t.Context(), `SELECT in_amount, in_mint FROM swaps WHERE source_id = $1
		ORDER BY created_at DESC, id DESC LIMIT 1`, cmd.ProposalID.UUID()).Scan(&in, &inMint); err != nil {
		t.Fatal(err)
	}
	if got := b.statuses(t, cmd); msg.verdict != "ack" || len(got) != 2 || got[1] != "confirmed" ||
		in != 50_000_000 || inMint != aaplxMint {
		t.Fatalf("verdict %q, swaps %v, new swap %d %s; want a confirmed sell of 50_000_000 AAPLx",
			msg.verdict, got, in, inMint)
	}
}
