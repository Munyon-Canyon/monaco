package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type TopReply = port.TopReply

func FindTopReply(
	ctx context.Context, db sqlc.DBTX, proposalIDs []ids.ProposalID, start, end time.Time,
) (TopReply, bool, error) {
	const op = "social.TopReply"
	if len(proposalIDs) == 0 {
		return TopReply{}, false, nil
	}
	raw := make([]uuid.UUID, len(proposalIDs))
	for i, id := range proposalIDs {
		raw[i] = id.UUID()
	}
	q := sqlc.New(db)
	row, err := q.TopReplyBetween(ctx, sqlc.TopReplyBetweenParams{ProposalIds: raw, WindowStart: start, WindowEnd: end})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return TopReply{}, false, nil
	case err != nil:
		return TopReply{}, false, errs.Wrap(err, errs.CodeInternal, op)
	}
	found, err := commentReactions(ctx, q, ids.UserID{}, []uuid.UUID{row.CommentID})
	if err != nil {
		return TopReply{}, false, err
	}
	top := found[row.CommentID].Reactions[0]
	body := domain.CommentBody(row.Body.String)
	return TopReply{
		CommentID: row.CommentID, ProposalID: row.ProposalID, AuthorID: ids.UserIDFrom(row.AuthorID),
		Excerpt: body.Excerpt(), ReactionCount: int(row.ReactionCount), TopEmoji: top.Emoji.String(),
		TopEmojiCount: int(top.Count),
	}, true, nil
}
