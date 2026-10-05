package adapters

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type Membership struct{}

func (Membership) Joined(ctx context.Context, tx db.Tx, e events.CabalMemberJoined, at time.Time) error {
	return trigger(ctx, tx, e.CabalID, "member_joined", at)
}

func (Membership) Left(ctx context.Context, tx db.Tx, e events.CabalMemberLeft, at time.Time) error {
	return trigger(ctx, tx, e.CabalID, "member_left", at)
}

func trigger(ctx context.Context, tx db.Tx, cabal uuid.UUID, reason string, at time.Time) error {
	return sqlc.New(tx.Queries()).InsertRankingTrigger(ctx, sqlc.InsertRankingTriggerParams{
		CabalID: cabal, Reason: reason, CreatedAt: at,
	})
}
