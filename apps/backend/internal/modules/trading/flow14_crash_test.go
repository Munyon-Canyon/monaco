//go:build faultpoints

package trading_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestFlow14_CashOut_CrashAfterSellRequest(t *testing.T) {
	t.Parallel()
	c := newCashOutEnv(t)
	cmd := app.SellForCashOut{
		CabalID: c.cabal, Source: domain.Source{Kind: domain.SourceCashout, ID: c.job},
		USDCNeeded: money.MicrosFromUint64(60_000_000),
	}
	testkit.CrashAt(t, faultpoint.AfterSellRequest, func(ctx context.Context) error {
		return c.handler().Handle(actorContext(ctx), cmd, nil)
	})
	assertLegs(t, c.legs(t), confirmedLeg("AAPLx", 1_000, 50_000_000), confirmedLeg("TSLAx", 106, 10_600_000))
	if c.positions != 1 {
		t.Fatalf("read positions %d times, want the redelivery to reuse the stored plan", c.positions)
	}
	for _, mint := range []string{aaplxMint, tslaxMint} {
		if sent := c.jup.Sent("req-" + mint); len(sent) != 1 {
			t.Fatalf("%s was sent %d times, want 1", mint, len(sent))
		}
	}
}
