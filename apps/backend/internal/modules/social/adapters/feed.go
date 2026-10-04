package adapters

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Feed struct {
	Bus   *bus.Conn
	Users app.Users
	IDs   ids.Generator
}

func (h Feed) Handle(ctx context.Context, tx db.Tx, e events.CabalCreated, at time.Time) error {
	name, err := h.FetchCreated(ctx, e)
	if err != nil {
		return err
	}
	return h.ApplyCreated(ctx, tx, e, name, at)
}

func (h Feed) FetchCreated(ctx context.Context, e events.CabalCreated) (string, error) {
	creator := ids.UserIDFrom(e.CreatorID)
	cards, err := h.Users.UsersByID(ctx, []ids.UserID{creator})
	if err != nil {
		return "", err
	}
	return cards[creator].DisplayName, nil
}

func (h Feed) ApplyCreated(ctx context.Context, tx db.Tx, e events.CabalCreated, name string, at time.Time) error {
	q := sqlc.New(tx.Queries())
	if err := q.UpsertFeedCabal(ctx, sqlc.UpsertFeedCabalParams{CabalID: e.CabalID, Name: e.Name, At: at}); err != nil {
		return err
	}
	if err := q.UpsertFeedMembership(ctx, sqlc.UpsertFeedMembershipParams{
		CabalID: e.CabalID, UserID: e.CreatorID, JoinedAt: at,
	}); err != nil {
		return err
	}
	faultpoint.Hit(ctx, faultpoint.BeforeCommit)
	return q.InsertFeedConsumerItem(ctx, sqlc.InsertFeedConsumerItemParams{
		ID: h.IDs.NewV7(), Kind: "cabal_created", RefType: "cabals", RefID: e.CabalID,
		CabalID: pgtype.UUID{Bytes: e.CabalID, Valid: true}, CabalName: pgtype.Text{String: e.Name, Valid: true},
		ActorID: pgtype.UUID{Bytes: e.CreatorID, Valid: true}, Title: name + " started " + e.Name,
		Payload: feed.Payload{CabalName: e.Name, ActorName: name}.JSON(), At: at,
	})
}
