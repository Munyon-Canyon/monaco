package market_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func droppedLines(t *testing.T, logs *testkit.Logs) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for line := range strings.Lines(string(logs.Bytes())) {
		var got map[string]any
		if json.Unmarshal([]byte(line), &got) == nil && got["msg"] == "market.prices.evm_dropped" {
			lines = append(lines, got)
		}
	}
	return lines
}

func TestSamplePrices_dropsAnEVMSampleMoreThanTheImpactLimitFromItsSolanaPrice(t *testing.T) {
	t.Parallel()
	source := &evmPrices{}
	r := newEVMRig(t, source)
	source.perShare[r.token] = money.MicrosFromUint64(257_600_000)
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))

	report, err := r.poller.Tick(ctx)
	if err != nil || attr(report.Attrs, "evm_priced") != "0" || attr(report.Attrs, "evm_gaps") != "AAPLxon@base" {
		t.Fatalf("Tick = %+v, %v, want the 3.04%% sample flagged and not stored", report, err)
	}
	wantPoints(t, r.pool, pricePoint{marketfake.AAPLx().Mint.String(), r.bucket, 250_000_000, "jupiter"})
	lines := droppedLines(t, logs)
	if len(lines) != 1 || lines[0]["reason"] != "off_reference" || lines[0]["symbol"] != "AAPLxon" ||
		lines[0]["network"] != "base" || lines[0]["level"] != "WARN" {
		t.Fatalf("dropped lines = %v, want one off_reference warning for AAPLxon on base", lines)
	}
}

func TestSamplePrices_logsAnUnquotedEVMListingOnceEveryTenMinutes(t *testing.T) {
	t.Parallel()
	r := newEVMRig(t, &evmPrices{})
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	for _, step := range []time.Duration{0, 9 * time.Minute, time.Minute} {
		r.clock.Advance(step)
		if _, err := r.poller.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	lines := droppedLines(t, logs)
	if len(lines) != 2 || lines[0]["reason"] != "no_quote" || lines[1]["listing"] != r.key {
		t.Fatalf("dropped lines = %v, want no_quote at 0 and 10 minutes, none at 9", lines)
	}
}
