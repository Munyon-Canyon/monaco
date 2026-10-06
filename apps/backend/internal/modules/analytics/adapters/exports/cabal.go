package exports

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Cabals struct{ Members app.MembershipReader }

func (c Cabals) CabalCreated(ctx context.Context, e events.CabalCreated) (app.Capture, bool, error) {
	return c.byMember(ctx, "cabal_created", e.CreatorID, map[string]any{
		"cabal_id": e.CabalID.String(), "join_mode": e.JoinMode, "threshold": e.Threshold,
	})
}

func (c Cabals) CabalJoined(ctx context.Context, e events.CabalMemberJoined) (app.Capture, bool, error) {
	return c.byMember(ctx, "cabal_joined", e.UserID, map[string]any{"cabal_id": e.CabalID.String(), "via": e.Via})
}

func (c Cabals) CabalLeft(ctx context.Context, e events.CabalMemberLeft) (app.Capture, bool, error) {
	return c.byMember(ctx, "cabal_left", e.UserID, map[string]any{"cabal_id": e.CabalID.String()})
}

func (c Cabals) byMember(
	ctx context.Context, event string, user uuid.UUID, props map[string]any,
) (app.Capture, bool, error) {
	cabals, err := c.Members.CabalsOf(ctx, ids.UserIDFrom(user))
	if err != nil {
		return app.Capture{}, false, errs.Wrap(err, errs.CodeDBUnavailable, "analytics.CabalCount")
	}
	return app.Capture{
		Event: event, DistinctID: user.String(), Properties: props,
		Set: map[string]any{"cabal_count": len(cabals)},
	}, true, nil
}
