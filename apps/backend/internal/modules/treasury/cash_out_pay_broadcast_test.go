package treasury_test

import (
	"log/slog"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestCashOutPayout_aBroadcastFailureLogsTheRPCMessage(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	message := "Transaction simulation failed: Error processing Instruction 0: custom program error: 0x1"
	r.transfers.sendErr = errs.New(errs.CodeInsufficientFunds, "solana.sendTransaction",
		slog.Int("rpc_code", -32002), slog.String("rpc_message", message))
	r.chain.set(payoutSig(1), landed())
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(observability.WithActor(r.f.ctx(), "system:treasury.cashout_payout"),
		observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	if err := r.payouts().Advance(ctx, r.job, 0, nil); err != nil {
		t.Fatal(err)
	}
	got := logs.Lines(observability.TreasuryCashOutBroadcastFailed.Name, "level", "code", "rpc_message")
	want := "WARN insufficient_funds " + message
	if len(got) != 1 || got[0] != want {
		t.Fatalf("broadcast_failed = %q, want one %q", got, want)
	}
}
