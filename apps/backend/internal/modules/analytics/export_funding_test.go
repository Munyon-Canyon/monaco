package analytics_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const depositActor = "system:poller.funding.deposits"

func depositOf(s moneyScene, address, signature string) events.DepositCredited {
	return events.DepositCredited{
		V: 1, DepositID: s.id, UserID: s.user, WalletAddress: chain.SolanaAddress(address),
		AmountMicros: money.MicrosFromUint64(27_500_001), TxSignature: chain.Signature(signature), Slot: 42,
	}
}

func TestAnalyticsExport_Funding(t *testing.T) {
	t.Parallel()
	exportsEachCaseOnce(t, analytics.RegisterFundingExports, map[string]moneyCase{
		"deposit.credited": {
			actor: depositActor,
			event: func(s moneyScene) events.Event { return depositOf(s, keyOf(7, 32), keyOf(9, 64)) },
			want: func(s moneyScene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event: "deposit_credited", DistinctID: s.user.String(),
					Properties: map[string]any{"deposit_id": s.id.String(), "usdc_amount": 27.500001},
				}
			},
		},
	})
}

func TestAnalyticsExport_Deposit_NoAddress(t *testing.T) {
	t.Parallel()
	e := newEnv(t, analytics.RegisterFundingExports)
	address, signature := keyOf(7, 32), keyOf(9, 64)
	ev := depositOf(newMoneyScene(e), address, signature)
	e.deliver(t, e.messageOf(ev.Type(), e.appendEvent(t, depositActor, ev)))
	captures := e.fake.Captures()
	if len(captures) != 1 {
		t.Fatalf("%d captures, want 1: %+v", len(captures), captures)
	}
	rendered := fmt.Sprintf("%+v", captures[0])
	for what, secret := range map[string]string{"wallet address": address, "signature": signature} {
		if strings.Contains(rendered, secret) {
			t.Errorf("the capture %s carries the %s", rendered, what)
		}
	}
}
