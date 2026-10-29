package app

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type CommentPreviews struct {
	db    sqlc.DBTX
	users Users
}

func NewCommentPreviews(db sqlc.DBTX, users Users) CommentPreviews {
	return CommentPreviews{db: db, users: users}
}

func (p CommentPreviews) CommentPreviews(
	ctx context.Context, viewer ids.UserID, commentIDs []uuid.UUID,
) (map[uuid.UUID]port.CommentPreview, error) {
	const op = "social.CommentPreviews"
	out := make(map[uuid.UUID]port.CommentPreview, len(commentIDs))
	if len(commentIDs) == 0 {
		return out, nil
	}
	rows, err := sqlc.New(p.db).CommentPreviews(ctx, sqlc.CommentPreviewsParams{
		CommentIds: commentIDs, Viewer: viewer.UUID(),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	authors := make([]ids.UserID, 0, len(rows))
	for _, row := range rows {
		author := ids.UserIDFrom(row.AuthorID)
		if !slices.Contains(authors, author) {
			authors = append(authors, author)
		}
	}
	cards, err := p.users.UsersByID(ctx, authors)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	found := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		found[i] = row.ID
	}
	reactions, err := commentReactions(ctx, sqlc.New(p.db), viewer, found)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		preview := previewOf(row, cards)
		summary := reactions[row.ID]
		preview.ReactionCount = summary.Total
		for _, r := range summary.Reactions {
			preview.Reactions = append(
				preview.Reactions,
				port.Reaction{Emoji: r.Emoji.String(), Count: r.Count, Mine: r.Mine},
			)
		}
		out[row.ID] = preview
	}
	return out, nil
}

func previewOf(row sqlc.CommentPreviewsRow, cards map[ids.UserID]UserCard) port.CommentPreview {
	author := ids.UserIDFrom(row.AuthorID)
	card := cards[author]
	preview := port.CommentPreview{
		FeedItemID: row.FeedObjectID, CommentID: row.ID, RootCommentID: row.RootCommentID,
		Author: port.CommentAuthor{ID: author, Name: card.DisplayName, PhotoURL: card.PhotoURL},
		Body:   row.Body.String, CreatedAt: row.CreatedAt,
	}
	if row.ParentCommentID.Valid {
		parent := uuid.UUID(row.ParentCommentID.Bytes)
		preview.ParentCommentID = &parent
	}
	if row.ReplyToCommentID.Valid {
		replyTo := uuid.UUID(row.ReplyToCommentID.Bytes)
		preview.ReplyToID = &replyTo
	}
	if row.OnProposal && row.CabalID.Valid {
		proposal, cabal := ids.ProposalIDFrom(row.RefID), ids.CabalIDFrom(row.CabalID.Bytes)
		preview.ProposalID, preview.CabalID = &proposal, &cabal
	}
	return preview
}
