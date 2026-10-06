package exports

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
)

func CabalFunded(_ context.Context, e events.Funded) (app.Capture, bool, error) {
	return byUser("cabal_funded", e.UserID, map[string]any{
		"cabal_id": e.CabalID.String(), "usdc_amount": usdc(e.AmountMicros),
	})
}

func CashOutCompleted(_ context.Context, e events.CashOutCompleted) (app.Capture, bool, error) {
	return byUser("cash_out_completed", e.UserID, map[string]any{
		"cabal_id": e.CabalID.String(), "usdc_amount": usdc(e.PayoutMicros),
	})
}

func CashOutPartial(_ context.Context, e events.CashOutPartial) (app.Capture, bool, error) {
	return byUser("cash_out_partial", e.UserID, map[string]any{
		"cabal_id": e.CabalID.String(), "usdc_amount": usdc(e.PayoutMicros),
	})
}

func CashOutFailed(_ context.Context, e events.CashOutFailed) (app.Capture, bool, error) {
	return byUser("cash_out_failed", e.UserID, map[string]any{"cabal_id": e.CabalID.String(), "code": e.Code})
}
