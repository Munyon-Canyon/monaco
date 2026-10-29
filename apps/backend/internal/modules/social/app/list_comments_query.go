package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	CommentPageDefault = 50
	CommentPageMax     = 100
)

type CommentsQuery struct {
	FeedObjectID uuid.UUID
	Viewer       ids.UserID
	After        *domain.Keyset
	Limit        int
	Stocks       StockCards
	Now          time.Time
}

type CommentThread struct {
	Comment Comment
	Replies []Comment
}

type CommentsPage struct {
	Threads []CommentThread
	Next    *domain.Keyset
}

func ListComments(ctx context.Context, db sqlc.DBTX, q CommentsQuery) (CommentsPage, error) {
	const op = "social.ListComments"
	if q.Limit < 1 || q.Limit > CommentPageMax {
		return CommentsPage{}, errs.New(errs.CodeInvalidInput, op, slog.Int("limit", q.Limit))
	}
	queries := sqlc.New(db)
	_, thread, err := ResolveThread(ctx, queries, q.FeedObjectID)
	if err != nil {
		return CommentsPage{}, err
	}
	params := sqlc.ListCommentThreadsParams{
		FeedObjectID: thread.ID, Viewer: q.Viewer.UUID(), RowLimit: int32(q.Limit) + 1,
	}
	if q.After != nil {
		params.HasCursor, params.AfterAt, params.AfterID = true, q.After.At, q.After.ID
	}
	rows, err := queries.ListCommentThreads(ctx, params)
	if err != nil {
		return CommentsPage{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	page := threadsOf(rows, q.Limit)
	if err := prependSystemThreads(ctx, queries, q, thread.ID, &page); err != nil {
		return CommentsPage{}, err
	}
	if err := attachCommentReactions(ctx, queries, q.Viewer, &page); err != nil {
		return CommentsPage{}, err
	}
	if err := attachStocks(ctx, q.Stocks, &page); err != nil {
		return CommentsPage{}, err
	}
	var listed []*Comment
	for i := range page.Threads {
		listed = append(listed, &page.Threads[i].Comment)
		for j := range page.Threads[i].Replies {
			listed = append(listed, &page.Threads[i].Replies[j])
		}
	}
	if err := pollViews(ctx, db, q.Viewer, listed, q.Now); err != nil {
		return CommentsPage{}, err
	}
	return page, nil
}

func prependSystemThreads(
	ctx context.Context, queries *sqlc.Queries, q CommentsQuery, itemID uuid.UUID, page *CommentsPage,
) error {
	if q.After != nil {
		return nil
	}
	system, err := SystemThreads(ctx, queries, itemID)
	if err != nil {
		return err
	}
	page.Threads = append(system, page.Threads...)
	return nil
}

func attachCommentReactions(ctx context.Context, reads *sqlc.Queries, viewer ids.UserID, page *CommentsPage) error {
	var commentIDs []uuid.UUID
	for _, thread := range page.Threads {
		commentIDs = append(commentIDs, thread.Comment.ID)
		for _, reply := range thread.Replies {
			commentIDs = append(commentIDs, reply.ID)
		}
	}
	found, err := commentReactions(ctx, reads, viewer, commentIDs)
	if err != nil {
		return err
	}
	for i := range page.Threads {
		thread := &page.Threads[i]
		top := found[thread.Comment.ID]
		thread.Comment.Reactions, thread.Comment.ReactionCount = top.Reactions, top.Total
		for j := range thread.Replies {
			reply := &thread.Replies[j]
			own := found[reply.ID]
			reply.Reactions, reply.ReactionCount = own.Reactions, own.Total
		}
	}
	return nil
}

func attachStocks(ctx context.Context, stocks StockCards, page *CommentsPage) error {
	var comments []*Comment
	var assetIDs []uuid.UUID
	for i := range page.Threads {
		thread := &page.Threads[i]
		all := []*Comment{&thread.Comment}
		for j := range thread.Replies {
			all = append(all, &thread.Replies[j])
		}
		for _, c := range all {
			if c.Attachment != nil && c.Attachment.Stock != nil {
				comments = append(comments, c)
				assetIDs = append(assetIDs, c.Attachment.Stock.AssetID)
			}
		}
	}
	if len(comments) == 0 {
		return nil
	}
	cards, err := stocks.StockCards(ctx, assetIDs)
	if err != nil {
		return errs.Wrap(err, errs.CodeOf(err), "social.ListComments")
	}
	for _, c := range comments {
		if card, ok := cards[c.Attachment.Stock.AssetID]; ok {
			c.Stock = &card
		}
	}
	return nil
}

func threadsOf(rows []sqlc.ListCommentThreadsRow, limit int) CommentsPage {
	var page CommentsPage
	index := map[uuid.UUID]int{}
	more := false
	for _, row := range rows {
		switch {
		case row.ParentCommentID.Valid:
			if i, ok := index[row.ParentCommentID.Bytes]; ok {
				page.Threads[i].Replies = append(page.Threads[i].Replies, listedComment(row))
			}
		case len(page.Threads) == limit:
			more = true
		default:
			index[row.ID] = len(page.Threads)
			page.Threads = append(page.Threads, CommentThread{Comment: listedComment(row)})
		}
	}
	if more {
		last := page.Threads[limit-1].Comment
		page.Next = &domain.Keyset{At: last.CreatedAt, ID: last.ID}
	}
	return page
}

func ProposalFeedItem(ctx context.Context, db sqlc.DBTX, proposal uuid.UUID) (uuid.UUID, error) {
	const op = "social.ProposalFeedItem"
	id, err := sqlc.New(db).GetProposalFeedItem(ctx, proposal)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return uuid.Nil, errs.New(errs.CodeFeedItemPending, op, slog.String("proposal_id", proposal.String()))
	case err != nil:
		return uuid.Nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	return id, nil
}

func listedComment(row sqlc.ListCommentThreadsRow) Comment {
	c := commentOf(sqlc.FeedComment{
		ID: row.ID, FeedObjectID: row.FeedObjectID, AuthorID: row.AuthorID, ParentCommentID: row.ParentCommentID,
		ReplyToUserID: row.ReplyToUserID, ReplyToCommentID: row.ReplyToCommentID, Body: row.Body,
		CreatedAt: row.CreatedAt, DeletedAt: row.DeletedAt, DeletedBy: row.DeletedBy, DeletedByKind: row.DeletedByKind,
	})
	c.Attachment = domain.DecodeAttachment(row.AttachmentKind.String, row.AttachmentPayload)
	if row.QuoteID.Valid {
		c.ReplyTo = quoteOf(
			row.QuoteID.Bytes, row.QuoteAuthorID.Bytes, row.QuoteParentID.Bytes, row.QuoteBody.String,
			row.QuoteHasAttachment, row.QuoteDeleted, row.QuoteBlocked,
		)
	}
	return c
}
