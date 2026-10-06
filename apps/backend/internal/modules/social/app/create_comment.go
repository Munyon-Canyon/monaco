package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type Comment struct {
	ID            uuid.UUID
	FeedObjectID  uuid.UUID
	AuthorID      ids.UserID
	ParentID      uuid.UUID
	ReplyToUserID ids.UserID
	Body          string
	CreatedAt     time.Time
	Deleted       bool
}

type CreateComment struct {
	FeedObjectID uuid.UUID
	Author       ids.UserID
	Body         domain.CommentBody
	ParentID     uuid.UUID
}

type CommentDeps struct {
	UoW     *db.UnitOfWork
	Reads   sqlc.DBTX
	Members Members
	IDs     ids.Generator
	Clock   clock.Clock
}

type CreateCommentHandler struct {
	d CommentDeps
}

func NewCreateCommentHandler(d CommentDeps) *CreateCommentHandler {
	return &CreateCommentHandler{d: d}
}

func (h *CreateCommentHandler) Handle(ctx context.Context, cmd CreateComment) (Comment, error) {
	const op = "social.CreateComment"
	item, err := sqlc.New(h.d.Reads).GetCommentItem(ctx, cmd.FeedObjectID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Comment{}, errs.New(
			errs.CodeFeedItemNotFound,
			op,
			slog.String("feed_object_id", cmd.FeedObjectID.String()),
		)
	case err != nil:
		return Comment{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	allowed, err := canComment(ctx, h.d.Members, feed.Kind(item.Kind), ids.CabalIDFrom(item.CabalID.Bytes), cmd.Author)
	if err != nil {
		return Comment{}, err
	}
	if !allowed {
		return Comment{}, errs.New(
			errs.CodeCommentMembersOnly,
			op,
			slog.String("feed_object_id", cmd.FeedObjectID.String()),
		)
	}
	var created Comment
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		target, place, err := loadParent(ctx, q, cmd)
		if err != nil {
			return err
		}
		row, err := q.InsertComment(ctx, sqlc.InsertCommentParams{
			ID: h.d.IDs.NewV7(), FeedObjectID: cmd.FeedObjectID, AuthorID: cmd.Author.UUID(),
			ParentCommentID: place.ParentID, ReplyToUserID: place.ReplyToID, Body: cmd.Body.String(),
			CreatedAt: h.d.Clock.Now().UTC(),
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		if _, err := q.BumpCommentCount(ctx, sqlc.BumpCommentCountParams{Delta: 1, ID: cmd.FeedObjectID}); err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		created = commentOf(row)
		return tx.Events.Append(ctx, createdEvent(item, row, target, cmd.Body))
	})
	if err != nil {
		return Comment{}, err
	}
	observability.Info(ctx, observability.SocialCommentCreated,
		slog.String("comment_id", created.ID.String()), slog.String("feed_object_id", created.FeedObjectID.String()))
	return created, nil
}

func createdEvent(
	item sqlc.GetCommentItemRow, row sqlc.InsertCommentRow, target sqlc.GetCommentTargetRow, body domain.CommentBody,
) events.CommentCreated {
	e := events.CommentCreated{
		V: 1, CommentID: row.ID, FeedObjectID: row.FeedObjectID, FeedKind: item.Kind, RefType: item.RefType,
		RefID: item.RefID, CabalID: parentOf(item.CabalID.Bytes), AuthorID: row.AuthorID,
		ParentCommentID: parentOf(row.ParentCommentID.Bytes), ParentAuthorID: parentOf(target.AuthorID),
		ParentDeleted: target.Deleted, ReplyToUserID: parentOf(row.ReplyToUserID.Bytes),
		ItemActorID: parentOf(item.ActorID.Bytes), Excerpt: body.Excerpt(),
	}
	if feed.Kind(item.Kind) == feed.KindProposal {
		e.ProposalID = parentOf(item.RefID)
	}
	return e
}

func commentOf(row sqlc.InsertCommentRow) Comment {
	return Comment{
		ID: row.ID, FeedObjectID: row.FeedObjectID, AuthorID: ids.UserIDFrom(row.AuthorID),
		ParentID: row.ParentCommentID.Bytes, ReplyToUserID: ids.UserIDFrom(row.ReplyToUserID.Bytes),
		Body: row.Body, CreatedAt: row.CreatedAt.UTC(),
	}
}

func canComment(
	ctx context.Context, members Members, kind feed.Kind, cabal ids.CabalID, user ids.UserID,
) (bool, error) {
	if kind != feed.KindProposal {
		return true, nil
	}
	member, err := members.IsMember(ctx, cabal, user)
	if err != nil {
		return false, errs.Wrap(err, errs.CodeOf(err), "social.canComment")
	}
	return member, nil
}

func loadParent(
	ctx context.Context, q *sqlc.Queries, cmd CreateComment,
) (sqlc.GetCommentTargetRow, domain.CommentPlacement, error) {
	const op = "social.CreateComment"
	if cmd.ParentID == uuid.Nil {
		return sqlc.GetCommentTargetRow{}, domain.CommentPlacement{}, nil
	}
	target, err := q.GetCommentTarget(
		ctx,
		sqlc.GetCommentTargetParams{ID: cmd.ParentID, FeedObjectID: cmd.FeedObjectID},
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return target, domain.CommentPlacement{},
			errs.New(errs.CodeCommentParentMismatch, op, slog.String("parent_comment_id", cmd.ParentID.String()))
	case err != nil:
		return target, domain.CommentPlacement{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return target, domain.PlaceReply(domain.CommentTarget{
		ID: target.ID, AuthorID: target.AuthorID, ParentID: target.ParentCommentID.Bytes,
	}), nil
}
