package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type BlockUser struct {
	Blocker ids.UserID
	Blocked ids.UserID
}

type BlockUserDeps struct {
	UoW   *db.UnitOfWork
	Users Users
	IDs   ids.Generator
	Clock clock.Clock
}

type BlockUserHandler struct {
	d BlockUserDeps
}

func NewBlockUserHandler(d BlockUserDeps) *BlockUserHandler { return &BlockUserHandler{d: d} }

func (h *BlockUserHandler) Handle(ctx context.Context, cmd BlockUser) error {
	const op = "social.BlockUser"
	if cmd.Blocker == cmd.Blocked {
		return errs.New(errs.CodeCannotBlockSelf, op)
	}
	status, err := statusOf(ctx, h.d.Users, cmd.Blocked)
	if err != nil {
		return err
	}
	if status == domain.AccountUnknown || status == domain.AccountDeleted {
		return errs.New(errs.CodeUserNotFound, op, slog.String("status", string(status)))
	}
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		now := h.d.Clock.Now().UTC()
		queries := sqlc.New(tx.Queries())
		id, err := queries.InsertUserBlock(ctx, sqlc.InsertUserBlockParams{
			ID: h.d.IDs.NewV7(), BlockerID: cmd.Blocker.UUID(), BlockedID: cmd.Blocked.UUID(), CreatedAt: now,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		for _, pair := range [][2]ids.UserID{{cmd.Blocker, cmd.Blocked}, {cmd.Blocked, cmd.Blocker}} {
			if err := removeFollow(ctx, tx, queries, pair[0], pair[1], now); err != nil {
				return err
			}
		}
		if err := tx.Events.Append(ctx, events.BlockCreated{
			V: 1, BlockID: id, BlockerID: cmd.Blocker.UUID(), BlockedID: cmd.Blocked.UUID(), CreatedAt: now,
		}); err != nil {
			return err
		}
		faultpoint.Hit(ctx, faultpoint.BeforeCommit)
		return nil
	})
}

func removeFollow(
	ctx context.Context, tx db.Tx, queries *sqlc.Queries, follower, followee ids.UserID, at time.Time,
) error {
	id, err := queries.RemoveFollow(ctx, sqlc.RemoveFollowParams{
		FollowerID: follower.UUID(), FolloweeID: followee.UUID(), RemovedAt: at,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "social.BlockUser")
	}
	return tx.Events.Append(ctx, events.FollowRemoved{
		V: 1, FollowID: id, FollowerID: follower.UUID(), FolloweeID: followee.UUID(), RemovedAt: at,
	})
}
