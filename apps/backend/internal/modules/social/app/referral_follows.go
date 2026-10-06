package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func MutuallyFollowable(ctx context.Context, users Users, a, b ids.UserID) (bool, error) {
	statusA, err := statusOf(ctx, users, a)
	if err != nil {
		return false, err
	}
	statusB, err := statusOf(ctx, users, b)
	if err != nil {
		return false, err
	}
	return domain.CanFollow(a, b, statusB) == nil && domain.CanFollow(b, a, statusA) == nil, nil
}

func FollowEachOther(
	ctx context.Context, tx db.Tx, gen ids.Generator, at time.Time, referrer, referee ids.UserID,
) error {
	for _, cmd := range []Follow{
		{Follower: referee, Followee: referrer, Source: domain.SourceReferral},
		{Follower: referrer, Followee: referee, Source: domain.SourceReferral},
	} {
		if err := CreateFollow(ctx, tx, gen, at, cmd); err != nil {
			return err
		}
	}
	return nil
}
