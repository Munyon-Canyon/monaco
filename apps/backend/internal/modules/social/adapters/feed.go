package adapters

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type Feed struct {
	Bus    *bus.Conn
	Users  app.Users
	Assets app.Assets
	IDs    ids.Generator
	UoW    *db.UnitOfWork
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
	return feedName(cards[creator].DisplayName, cards[creator].Handle), nil
}

func (h Feed) ApplyCreated(ctx context.Context, tx db.Tx, e events.CabalCreated, name string, at time.Time) error {
	q := sqlc.New(tx.Queries())
	if err := q.UpsertFeedCabal(ctx, sqlc.UpsertFeedCabalParams{CabalID: e.CabalID, Name: e.Name, At: at}); err != nil {
		return err
	}
	if _, err := q.UpsertFeedMembership(ctx, sqlc.UpsertFeedMembershipParams{
		CabalID: e.CabalID, UserID: e.CreatorID, JoinedAt: at, EventID: eventID(ctx),
	}); err != nil {
		return err
	}
	faultpoint.Hit(ctx, faultpoint.BeforeCommit)
	if err := q.InsertFeedConsumerItem(ctx, sqlc.InsertFeedConsumerItemParams{
		ID: eventID(ctx), Kind: "cabal_created", RefType: "cabals", RefID: e.CabalID,
		CabalID: pgtype.UUID{Bytes: e.CabalID, Valid: true}, CabalName: pgtype.Text{String: e.Name, Valid: true},
		ActorID: pgtype.UUID{Bytes: e.CreatorID, Valid: true}, Title: name + " started " + e.Name,
		Payload: feed.Payload{CabalName: e.Name, ActorName: name}.JSON(), At: at,
	}); err != nil {
		return err
	}
	tx.AfterCommit(func(ctx context.Context) { h.Bus.PublishHint(ctx, "global.feed", nil) })
	return nil
}

func (h Feed) Joined(ctx context.Context, tx db.Tx, e events.CabalMemberJoined, at time.Time) error {
	name, err := h.FetchJoined(ctx, e)
	if err != nil {
		return err
	}
	return h.ApplyJoined(ctx, tx, e, name, at)
}

func (h Feed) FetchJoined(ctx context.Context, e events.CabalMemberJoined) (string, error) {
	if e.Via == "create" {
		return "", nil
	}
	member := ids.UserIDFrom(e.UserID)
	cards, err := h.Users.UsersByID(ctx, []ids.UserID{member})
	if err != nil {
		return "", err
	}
	return feedName(cards[member].DisplayName, cards[member].Handle), nil
}

func (h Feed) ApplyJoined(
	ctx context.Context, tx db.Tx, e events.CabalMemberJoined, name string, at time.Time,
) error {
	q := sqlc.New(tx.Queries())
	exists, err := q.FeedCabalExists(ctx, e.CabalID)
	if err != nil {
		return err
	}
	if !exists {
		return errs.New(errs.CodeFeedItemPending, "social.Feed.Joined")
	}
	if _, err := q.UpsertFeedMembership(ctx, sqlc.UpsertFeedMembershipParams{
		CabalID: e.CabalID, UserID: e.UserID, JoinedAt: at, EventID: eventID(ctx),
	}); err != nil {
		return err
	}
	if e.Via == "create" {
		return nil
	}
	cabal, err := q.FeedCabalName(ctx, e.CabalID)
	if err != nil {
		return err
	}
	if err := q.InsertFeedConsumerItem(ctx, sqlc.InsertFeedConsumerItemParams{
		ID: eventID(ctx), Kind: "member_joined", RefType: "cabal_members", RefID: eventID(ctx),
		CabalID: pgtype.UUID{Bytes: e.CabalID, Valid: true}, CabalName: pgtype.Text{String: cabal, Valid: true},
		ActorID: pgtype.UUID{Bytes: e.UserID, Valid: true}, Title: name + " joined " + cabal,
		Payload: feed.Payload{CabalName: cabal, ActorName: name}.JSON(), At: at,
	}); err != nil {
		return err
	}
	tx.AfterCommit(func(ctx context.Context) { h.Bus.PublishHint(ctx, "global.feed", nil) })
	return nil
}

func (Feed) Left(ctx context.Context, tx db.Tx, e events.CabalMemberLeft, at time.Time) error {
	q := sqlc.New(tx.Queries())
	exists, err := q.FeedCabalExists(ctx, e.CabalID)
	if err != nil {
		return err
	}
	if !exists {
		return errs.New(errs.CodeFeedItemPending, "social.Feed.Left")
	}
	_, err = q.DeleteFeedMembership(ctx, sqlc.DeleteFeedMembershipParams{
		CabalID: e.CabalID, UserID: e.UserID, EventID: eventID(ctx), At: at,
	})
	return err
}

func feedName(displayName, handle string) string {
	if displayName != "" {
		return displayName
	}
	return handle
}

func eventID(ctx context.Context) [16]byte {
	id, _ := ids.ParseEventID(observability.EventIDFrom(ctx))
	return id.UUID()
}
