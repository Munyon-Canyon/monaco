package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (f Follows) BlockedByMe(ctx context.Context, viewer, user ids.UserID) (bool, error) {
	const op = "social.FollowsPort.BlockedByMe"
	blocked, err := sqlc.New(f.db).IsBlocked(ctx, sqlc.IsBlockedParams{Blocker: viewer.UUID(), Blocked: user.UUID()})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeOf(err), op)
	}
	return blocked, nil
}
