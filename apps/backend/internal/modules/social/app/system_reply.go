package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Holdings interface {
	Posted(ctx context.Context, swapID uuid.UUID) (bool, error)
	Holding(ctx context.Context, cabalID ids.CabalID, mint chain.SolanaAddress) (uint64, error)
}

type SystemReply struct {
	ItemID  uuid.UUID
	Outcome domain.SystemOutcome
	Side    feed.Action
	Symbol  string
	Before  *money.BaseUnits
	After   *money.BaseUnits
	At      time.Time
}

func WriteSystemReply(ctx context.Context, tx db.Tx, hints Hints, r SystemReply) error {
	const op = "social.WriteSystemReply"
	q := sqlc.New(tx.Queries())
	rows, err := q.InsertSystemReply(ctx, sqlc.InsertSystemReplyParams{
		ID: uuid.NewSHA1(r.ItemID, []byte(r.Outcome)), FeedObjectID: r.ItemID, Outcome: string(r.Outcome),
		Body: domain.SystemReplyText(r.Outcome, r.Side, r.Symbol, r.Before, r.After), CreatedAt: r.At.UTC(),
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	if rows == 0 {
		return nil
	}
	if _, err := q.BumpCommentCount(ctx, sqlc.BumpCommentCountParams{Delta: 1, ID: r.ItemID}); err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	tx.AfterCommit(func(ctx context.Context) { hints.PublishHint(ctx, FeedHint, nil) })
	return nil
}

func SystemThreads(ctx context.Context, q *sqlc.Queries, itemID uuid.UUID) ([]CommentThread, error) {
	rows, err := q.ListSystemReplies(ctx, itemID)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "social.SystemThreads")
	}
	threads := make([]CommentThread, len(rows))
	for i, row := range rows {
		threads[i] = CommentThread{Comment: commentOf(row)}
	}
	return threads, nil
}

func refuseSystemReply(kind, op string, id uuid.UUID) error {
	if kind != domain.AuthorKindSystem {
		return nil
	}
	return errs.New(errs.CodeInvalidInput, op,
		slog.String("reason", domain.ReasonSystemReply), slog.String("comment_id", id.String()))
}
