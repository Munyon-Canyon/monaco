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
		blocked, err := blockedEitherWay(ctx, tx, cmd.Follower, cmd.Followee)
		if err != nil {
			return err
		}
		if blocked {
			return errs.New(errs.CodeFollowBlocked, "social.Follow")
		}
		return CreateFollow(ctx, tx, h.d.IDs, h.d.Clock.Now().UTC(), cmd)
	})
}

func lockPair(ctx context.Context, queries *sqlc.Queries, a, b ids.UserID) error {
	err := queries.LockUserPair(ctx, sqlc.LockUserPairParams{UserA: a.UUID(), UserB: b.UUID()})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "social.LockUserPair")
	}
	return nil
}

func blockedEitherWay(ctx context.Context, tx db.Tx, a, b ids.UserID) (bool, error) {
	queries := sqlc.New(tx.Queries())
	if err := lockPair(ctx, queries, a, b); err != nil {
		return false, err
	}
	blocked, err := queries.IsBlockedEitherWay(ctx, sqlc.IsBlockedEitherWayParams{UserA: a.UUID(), UserB: b.UUID()})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, "social.Follow")
	}
	return blocked, nil
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
