package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type FeedItemView struct {
	Item    FeedItem
	Visible bool
}

func GetFeedItem(
	ctx context.Context, db sqlc.DBTX, viewer ids.UserID, id uuid.UUID, filter FeedFilter,
) (FeedItemView, error) {
	const op = "social.GetFeedItem"
	f := filterParams(viewer, filter)
	row, err := sqlc.New(db).GetFeedItem(ctx, sqlc.GetFeedItemParams{
		Kinds: f.Kinds, CabalID: f.CabalID, Symbol: f.Symbol, Q: f.Q, Following: f.Following, Mine: f.Mine,
		Viewer: f.Viewer, ID: id,
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return FeedItemView{}, errs.New(errs.CodeFeedItemNotFound, op)
	case err != nil:
		return FeedItemView{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	item, err := feedItemOf(sqlc.ListFeedRow{
		ID: row.ID, Kind: row.Kind, RefType: row.RefType, RefID: row.RefID, CabalID: row.CabalID,
		ActorID: row.ActorID, Symbol: row.Symbol, Title: row.Title, Body: row.Body, Payload: row.Payload,
		Status: row.Status, CommentCount: row.CommentCount, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	})
	if err != nil {
		return FeedItemView{}, err
	}
	return FeedItemView{Item: item, Visible: row.Visible}, nil
}
