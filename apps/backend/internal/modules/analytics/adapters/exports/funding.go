package exports

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
)

func DepositCredited(_ context.Context, e events.DepositCredited) (app.Capture, bool, error) {
	return byUser("deposit_credited", e.UserID, map[string]any{
		"deposit_id": e.DepositID.String(), "usdc_amount": usdc(e.AmountMicros),
	})
}

func byUser(event string, user uuid.UUID, props map[string]any) (app.Capture, bool, error) {
	return app.Capture{Event: event, DistinctID: user.String(), Properties: props}, true, nil
}
