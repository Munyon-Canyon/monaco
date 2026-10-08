package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Follow struct {
	Follower ids.UserID
	Followee ids.UserID
	Source   domain.FollowSource
}

type FollowDeps struct {
	UoW   *db.UnitOfWork
	Users Users
	IDs   ids.Generator
	Clock clock.Clock
}

type FollowHandler struct {
	d FollowDeps
}

func NewFollowHandler(d FollowDeps) *FollowHandler { return &FollowHandler{d: d} }

func (h *FollowHandler) Handle(ctx context.Context, cmd Follow) error {
	status, err := statusOf(ctx, h.d.Users, cmd.Followee)
	if err != nil {
		return err
	}
	if err := domain.CanFollow(cmd.Follower, cmd.Followee, status); err != nil {
		return err
	}
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		blocked, err := sqlc.New(tx.Queries()).IsBlockedEitherWay(ctx, sqlc.IsBlockedEitherWayParams{
			UserA: cmd.Follower.UUID(), UserB: cmd.Followee.UUID(),
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, "social.Follow")
		}
		if blocked {
			return errs.New(errs.CodeFollowBlocked, "social.Follow")
		}
		return CreateFollow(ctx, tx, h.d.IDs, h.d.Clock.Now().UTC(), cmd)
	})
}

func CreateFollow(ctx context.Context, tx db.Tx, gen ids.Generator, at time.Time, cmd Follow) error {
	id, err := sqlc.New(tx.Queries()).InsertFollow(ctx, sqlc.InsertFollowParams{
		ID: gen.NewV7(), FollowerID: cmd.Follower.UUID(), FolloweeID: cmd.Followee.UUID(),
		Source: cmd.Source.String(), CreatedAt: at,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "social.Follow")
	}
	return tx.Events.Append(ctx, events.FollowCreated{
		V: 1, FollowID: id, FollowerID: cmd.Follower.UUID(), FolloweeID: cmd.Followee.UUID(),
		Source: cmd.Source.String(), CreatedAt: at,
	})
}
