package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ReferralFollows struct {
	Users app.Users
	IDs   ids.Generator
}

func (h ReferralFollows) Fetch(ctx context.Context, e events.ReferralAttributed) (bool, error) {
	return app.MutuallyFollowable(ctx, h.Users, ids.UserIDFrom(e.Referrer), ids.UserIDFrom(e.Referee))
}

func (h ReferralFollows) Apply(
	ctx context.Context, tx db.Tx, e events.ReferralAttributed, followable bool, at time.Time,
) error {
	if !followable {
		return nil
	}
	return app.FollowEachOther(ctx, tx, h.IDs, at, ids.UserIDFrom(e.Referrer), ids.UserIDFrom(e.Referee))
}
