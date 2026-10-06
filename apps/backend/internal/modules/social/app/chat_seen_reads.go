package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ChatSeenByQuery struct {
	CabalID   ids.CabalID
	Viewer    ids.UserID
	MessageID uuid.UUID
}

type SeenBy struct {
	Members []ids.UserID
}

func GetChatSeenBy(ctx context.Context, db sqlc.DBTX, members Members, q ChatSeenByQuery) (SeenBy, error) {
	const op = "social.GetChatSeenBy"
	if err := requireMember(ctx, members, q.CabalID, q.Viewer); err != nil {
		return SeenBy{}, err
	}
	reads := sqlc.New(db)
	message, err := reads.GetChatMessage(ctx, sqlc.GetChatMessageParams{ID: q.MessageID, CabalID: q.CabalID.UUID()})
	if errors.Is(err, sql.ErrNoRows) {
		return SeenBy{}, errs.New(errs.CodeChatMessageNotFound, op, slog.String("message_id", q.MessageID.String()))
	}
	if err != nil {
		return SeenBy{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	seen, err := reads.ListSeenBy(ctx, sqlc.ListSeenByParams{
		CabalID: q.CabalID.UUID(), AuthorID: message.AuthorID, MessageAt: message.CreatedAt,
	})
	if err != nil {
		return SeenBy{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	out := SeenBy{Members: make([]ids.UserID, len(seen))}
	for i, id := range seen {
		out.Members[i] = ids.UserIDFrom(id)
	}
	return out, nil
}

func withSeenCount(ctx context.Context, reads *sqlc.Queries, cabal ids.CabalID, page []ChatMessage) error {
	newest, err := reads.NewestSeenCount(ctx, cabal.UUID())
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "social.withSeenCount")
	}
	for i := range page {
		if page[i].ID == newest.MessageID {
			count := int(newest.SeenCount)
			page[i].SeenCount = &count
		}
	}
	return nil
}
