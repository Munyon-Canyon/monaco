package analytics_test

import (
	"maps"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func cabalWants(event string, extra map[string]any) func(s moneyScene) fakes.PostHogCapture {
	return func(s moneyScene) fakes.PostHogCapture {
		props := map[string]any{"cabal_id": s.cabal.String()}
		maps.Copy(props, extra)
		return fakes.PostHogCapture{Event: event, DistinctID: s.user.String(), Properties: props}
	}
}

func TestAnalyticsExport_Treasury(t *testing.T) {
	t.Parallel()
	exportsEachCaseOnce(t, analytics.RegisterTreasuryExports, map[string]moneyCase{
		"cabal.funded": {
			actor: "system:poller.treasury.fund-transfers",
			event: func(s moneyScene) events.Event {
				return events.Funded{
					V: 1, TransferID: s.id, CabalID: s.cabal, UserID: s.user,
					AmountMicros: money.MicrosFromUint64(5_250_001), ShareUnits: money.SharesUnitsFromUint64(5_000_000),
					SharePriceMicros:     money.MicrosFromUint64(1_050_000),
					PotValueBeforeMicros: money.MicrosFromUint64(20_000_000),
					TotalSharesAfter:     money.SharesUnitsFromUint64(25_000_000),
					TxSignature:          chain.Signature(keyOf(9, 64)),
				}
			},
			want: cabalWants("cabal_funded", map[string]any{"usdc_amount": 5.250001}),
		},
		"cashout.completed": {
			actor: "system:treasury.cashout_payout",
			event: func(s moneyScene) events.Event {
				return events.CashOutCompleted{
					V: 1, JobID: s.id, CabalID: s.cabal, UserID: s.user, ShareUnits: 2_000_000,
					PayoutMicros: money.MicrosFromUint64(1_234_567), Signature: chain.Signature(keyOf(9, 64)),
				}
			},
			want: cabalWants("cash_out_completed", map[string]any{"usdc_amount": 1.234567}),
		},
		"cashout.partial": {
			actor: "system:treasury.cashout_payout",
			event: func(s moneyScene) events.Event {
				return events.CashOutPartial{
					V: 1, JobID: s.id, CabalID: s.cabal, UserID: s.user, ShareUnitsBurned: 1_500_000,
					ShareUnitsReturned: 500_000, PayoutMicros: money.MicrosFromUint64(750_000),
					Signature: chain.Signature(keyOf(9, 64)),
				}
			},
			want: cabalWants("cash_out_partial", map[string]any{"usdc_amount": 0.75}),
		},
		"cashout.failed": {
			actor: "system:treasury.cashout_payout",
			event: func(s moneyScene) events.Event {
				return events.CashOutFailed{
					V: 1, JobID: s.id, CabalID: s.cabal, UserID: s.user, ShareUnits: 2_000_000,
					Code: string(errs.CodePayoutFailed),
				}
			},
			want: cabalWants("cash_out_failed", map[string]any{"code": "payout_failed"}),
		},
	})
}
