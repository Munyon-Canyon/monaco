package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Unfollow struct {
	Follower ids.UserID
	Followee ids.UserID
}

type UnfollowHandler struct {
	uow   *db.UnitOfWork
	clock clock.Clock
}

func NewUnfollowHandler(uow *db.UnitOfWork, clk clock.Clock) *UnfollowHandler {
	return &UnfollowHandler{uow: uow, clock: clk}
}

func (h *UnfollowHandler) Handle(ctx context.Context, cmd Unfollow) error {
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		now := h.clock.Now().UTC()
		id, err := sqlc.New(tx.Queries()).RemoveFollow(ctx, sqlc.RemoveFollowParams{
			FollowerID: cmd.Follower.UUID(), FolloweeID: cmd.Followee.UUID(), RemovedAt: now,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, "social.Unfollow")
		}
		return tx.Events.Append(ctx, events.FollowRemoved{
			V: 1, FollowID: id, FollowerID: cmd.Follower.UUID(), FolloweeID: cmd.Followee.UUID(), RemovedAt: now,
		})
	})
}
