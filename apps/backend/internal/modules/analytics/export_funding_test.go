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

const (
	depositActor    = "system:poller.funding.deposits"
	onrampActor     = "system:funding.onramp"
	withdrawalActor = "system:poller.funding.withdrawals"
)

func depositOf(s moneyScene, address, signature string) events.Event {
	return events.DepositCredited{
		V: 1, DepositID: s.id, UserID: s.user, WalletAddress: chain.SolanaAddress(address),
		AmountMicros: money.MicrosFromUint64(27_500_001), TxSignature: chain.Signature(signature), Slot: 42,
	}
}

func onrampOf(s moneyScene, from *string, to string) events.Event {
	return events.OnrampStatusChanged{V: 1, SessionID: s.id, UserID: s.user, From: from, To: to}
}

func withdrawalOf(s moneyScene, toAddress, signature string) events.Event {
	return events.WithdrawalConfirmed{
		V: 1, WithdrawalID: s.id, UserID: s.user, AmountMicros: money.MicrosFromUint64(2_000_001),
		ToAddress: chain.SolanaAddress(toAddress), TxSignature: chain.Signature(signature),
	}
}

func onrampWants(from any, to string) func(s moneyScene) fakes.PostHogCapture {
	return func(s moneyScene) fakes.PostHogCapture {
		return fakes.PostHogCapture{
			Event: "onramp_status_changed", DistinctID: s.user.String(),
			Properties: map[string]any{"session_id": s.id.String(), "from": from, "to": to},
		}
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
		"onramp.status_changed first status": {
			actor: onrampActor,
			event: func(s moneyScene) events.Event { return onrampOf(s, nil, "created") },
			want:  onrampWants(nil, "created"),
		},
		"onramp.status_changed later status": {
			actor: onrampActor,
			event: func(s moneyScene) events.Event {
				opened := "opened"
				return onrampOf(s, &opened, "confirmed")
			},
			want: onrampWants("opened", "confirmed"),
		},
		"withdrawal.confirmed": {
			actor: withdrawalActor,
			event: func(s moneyScene) events.Event { return withdrawalOf(s, keyOf(5, 32), keyOf(9, 64)) },
			want: func(s moneyScene) fakes.PostHogCapture {
				return fakes.PostHogCapture{
					Event: "withdrawal_sent", DistinctID: s.user.String(),
					Properties: map[string]any{"withdrawal_id": s.id.String(), "usdc_amount": 2.000001},
				}
			},
		},
	})
}

func TestAnalyticsExport_Deposit_NoAddress(t *testing.T) {
	t.Parallel()
	exportsNoSecret(t, depositActor, depositOf)
}

func TestAnalyticsExport_Withdrawal_NoAddress(t *testing.T) {
	t.Parallel()
	exportsNoSecret(t, withdrawalActor, withdrawalOf)
}

func exportsNoSecret(
	t *testing.T, actor string, event func(s moneyScene, address, signature string) events.Event,
) {
	t.Helper()
	e := newEnv(t, analytics.RegisterFundingExports)
	address, signature := keyOf(7, 32), keyOf(9, 64)
	ev := event(newMoneyScene(e), address, signature)
	e.deliver(t, e.messageOf(ev.Type(), e.appendEvent(t, actor, ev)))
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
