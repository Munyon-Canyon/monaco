package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
)

const (
	CommentPageDefault = 50
	CommentPageMax     = 100
)

type CommentsQuery struct {
	FeedObjectID uuid.UUID
	After        *domain.Keyset
	Limit        int
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
	_, err := queries.GetCommentItem(ctx, q.FeedObjectID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return CommentsPage{}, errs.New(errs.CodeFeedItemNotFound, op,
			slog.String("feed_object_id", q.FeedObjectID.String()))
	case err != nil:
		return CommentsPage{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	params := sqlc.ListCommentThreadsParams{FeedObjectID: q.FeedObjectID, RowLimit: int32(q.Limit) + 1}
	if q.After != nil {
		params.HasCursor, params.AfterAt, params.AfterID = true, q.After.At, q.After.ID
	}
	rows, err := queries.ListCommentThreads(ctx, params)
	if err != nil {
		return CommentsPage{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return threadsOf(rows, q.Limit), nil
}

func threadsOf(rows []sqlc.FeedComment, limit int) CommentsPage {
	var page CommentsPage
	index := map[uuid.UUID]int{}
	more := false
	for _, row := range rows {
		switch {
		case row.ParentCommentID.Valid:
			if i, ok := index[row.ParentCommentID.Bytes]; ok {
				page.Threads[i].Replies = append(page.Threads[i].Replies, commentOf(row))
			}
		case len(page.Threads) == limit:
			more = true
		default:
			index[row.ID] = len(page.Threads)
			page.Threads = append(page.Threads, CommentThread{Comment: commentOf(row)})
		}
	}
	if more {
		last := page.Threads[limit-1].Comment
		page.Next = &domain.Keyset{At: last.CreatedAt, ID: last.ID}
	}
	return page
}
