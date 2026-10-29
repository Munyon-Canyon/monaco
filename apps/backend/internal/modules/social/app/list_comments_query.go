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
	return threadsOf(rows, q.Limit), nil
}

func threadsOf(rows []sqlc.ListCommentThreadsRow, limit int) CommentsPage {
	var page CommentsPage
	index := map[uuid.UUID]int{}
	more := false
	for _, row := range rows {
		switch {
		case row.ParentCommentID.Valid:
			if i, ok := index[row.ParentCommentID.Bytes]; ok {
				page.Threads[i].Replies = append(page.Threads[i].Replies, threadCommentOf(row))
			}
		case len(page.Threads) == limit:
			more = true
		default:
			index[row.ID] = len(page.Threads)
			page.Threads = append(page.Threads, CommentThread{Comment: threadCommentOf(row)})
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

func threadCommentOf(row sqlc.ListCommentThreadsRow) Comment {
	c := commentOf(sqlc.FeedComment{
		ID: row.ID, FeedObjectID: row.FeedObjectID, AuthorID: row.AuthorID, ParentCommentID: row.ParentCommentID,
		ReplyToUserID: row.ReplyToUserID, Body: row.Body, CreatedAt: row.CreatedAt, DeletedAt: row.DeletedAt,
		DeletedBy: row.DeletedBy, DeletedByKind: row.DeletedByKind, ReplyToCommentID: row.ReplyToCommentID,
	})
	if row.ReplyToCommentID.Valid {
		c.Quote = quoteSource{
			ID: row.ReplyToCommentID.Bytes, AuthorID: row.QuoteAuthorID.Bytes, ParentID: row.QuoteParentID.Bytes,
			Body: row.QuoteBody.String, Deleted: row.QuoteDeleted, Blocked: row.QuoteBlocked,
		}.quote()
	}
	return c
}
