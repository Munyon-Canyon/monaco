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
	reads := sqlc.New(db)
	row, err := reads.GetFeedItem(ctx, sqlc.GetFeedItemParams{
		Kinds: f.Kinds, CabalID: f.CabalID, ActorID: f.ActorID, Symbol: f.Symbol, Q: f.Q, Following: f.Following,
		Mine: f.Mine, Network: f.Network, Others: f.Others, Viewer: f.Viewer, ID: id, ThreadKinds: f.ThreadKinds,
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
		Status: row.Status, AssetID: row.AssetID, CabalName: row.CabalName,
		CommentCount: row.CommentCount, ThreadItemID: row.ThreadItemID, ReplyCount: row.ReplyCount,
		ThreadKind: row.ThreadKind, ThreadCabalName: row.ThreadCabalName, MemberOfThreadCabal: row.MemberOfThreadCabal,
		LikeCount: row.LikeCount, LikedByMe: row.LikedByMe,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	})
	if err != nil {
		return FeedItemView{}, err
	}
	item.CanReact = row.Visible
	items := []FeedItem{item}
	if err := attachReactions(ctx, reads, viewer, items); err != nil {
		return FeedItemView{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	return FeedItemView{Item: items[0], Visible: row.Visible}, nil
}
