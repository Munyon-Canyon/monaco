package exports

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
)

func ReferralAttributed(_ context.Context, e events.ReferralAttributed) (app.Capture, bool, error) {
	return byUser("referral_attributed", e.Referee, map[string]any{
		"source": e.Source, "referrer_id": e.Referrer.String(),
	})
}

func ReferralQualified(_ context.Context, e events.ReferralQualified) (app.Capture, bool, error) {
	return byUser("referral_qualified", e.Referee, map[string]any{"referrer_id": e.Referrer.String()})
}
