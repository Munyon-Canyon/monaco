package adapters

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
)

type FeedStore struct {
	q *sqlc.Queries
}

func NewFeedStore(db sqlc.DBTX) FeedStore { return FeedStore{q: sqlc.New(db)} }

func (s FeedStore) UpsertItem(ctx context.Context, id uuid.UUID, item feed.Item, at time.Time) (uuid.UUID, error) {
	const op = "social.FeedStore.UpsertItem"
	stored, err := s.q.UpsertFeedItem(ctx, sqlc.UpsertFeedItemParams{
		ID: id, Kind: string(item.Kind), RefType: string(item.Kind.RefType()), RefID: item.RefID,
		CabalID: optionalID(item.CabalID.UUID()), CabalName: optionalText(item.Payload.CabalName),
		ActorID: optionalID(item.ActorID.UUID()), AssetID: optionalID(item.AssetID),
		Symbol: optionalText(item.Payload.Symbol), Title: feed.RenderTitle(item.Kind, item.Payload),
		Body: optionalText(item.Body), Payload: item.Payload.JSON(), Status: optionalText(item.Status), At: at,
	})
	if err != nil {
		return uuid.Nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	return stored, nil
}

func (s FeedStore) UpdateStatus(ctx context.Context, change feed.StatusChange, at time.Time) (bool, error) {
	const op = "social.FeedStore.UpdateStatus"
	n, err := s.q.UpdateFeedStatus(ctx, sqlc.UpdateFeedStatusParams{
		ToStatus: change.To, FromStatus: change.From, Title: feed.RenderTitle(change.Kind, change.Payload),
		Payload: change.Payload.JSON(), At: at, RefType: string(change.Kind.RefType()), RefID: change.RefID,
		Kind: string(change.Kind),
	})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeOf(err), op)
	}
	return n == 1, nil
}

func optionalID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil}
}

func optionalText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}
