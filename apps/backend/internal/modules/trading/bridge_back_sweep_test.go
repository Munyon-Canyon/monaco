package trading_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func TestEVMTrades_aFailedBridgeBackIsAlertedAndSentAgainUnderTheNextAttemptKey(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.failedSale(t, false, multichain.ActionRejected)
	var logs bytes.Buffer
	ctx := observability.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&logs, nil)))

	if _, err := e.tick(ctx, t); err != nil || logs.Len() != 0 {
		t.Fatalf("a young failure ticked with %v and logs %s, want it left alone", err, logs.String())
	}
	e.clk.Advance(app.SwapSweepAge)
	if _, err := e.tick(ctx, t); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "trading.bridge_back.failed") ||
		!strings.Contains(logs.String(), `"level":"ERROR"`) ||
		!strings.Contains(logs.String(), `"retrying":true`) {
		t.Fatalf("logs = %s, want an alert for the failed return that says it is retrying", logs.String())
	}
	if attempt, prior, actions := e.attempts(t, swap); attempt != 2 || prior != "privy-action-1" || actions != 1 {
		t.Fatalf(
			"attempt %d after %s with %d operator actions, want attempt 2 after privy-action-1",
			attempt,
			prior,
			actions,
		)
	}
	e.privy.script(multichain.ActionRejected, multichain.ActionSucceeded)
	e.clk.Advance(app.SwapSweepAge + 1)
	e.mustTick(t, 1, 1)
	if status, action := e.bridgeBackOf(t, swap); status != "succeeded" || action != "privy-action-2" {
		t.Fatalf("bridge_back %s on %s, want succeeded on privy-action-2", status, action)
	}
}

func TestEVMTrades_aBridgeBackThatKeepsFailingStopsRetryingButKeepsAlerting(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.failedSale(t, false, multichain.ActionRejected)
	if _, err := e.pool.Exec(t.Context(),
		`UPDATE trade_steps SET attempt = $2 WHERE trade_id = $1 AND step = 'bridge_back'`,
		swap, app.MaxBridgeBackAttempts); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	ctx := observability.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&logs, nil)))

	for range 2 {
		e.clk.Advance(app.SwapSweepAge + 1)
		if _, err := e.tick(ctx, t); err != nil {
			t.Fatal(err)
		}
	}
	attempt, _, actions := e.attempts(t, swap)
	if status, _ := e.bridgeBackOf(
		t,
		swap,
	); status != "failed" || attempt != app.MaxBridgeBackAttempts ||
		actions != 0 ||
		strings.Count(logs.String(), `"retrying":false`) != 2 {
		t.Fatalf(
			"bridge_back %s at attempt %d with %d retries and logs %s, want it left failed and alerted on each tick",
			status,
			attempt,
			actions,
			logs.String(),
		)
	}
}
