package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
)

type CommentLiveness struct{ db sqlc.DBTX }

func NewCommentLiveness(db sqlc.DBTX) CommentLiveness { return CommentLiveness{db: db} }

func (c CommentLiveness) CommentDeleted(ctx context.Context, id uuid.UUID) (bool, error) {
	ref, err := sqlc.New(c.db).GetCommentRef(ctx, id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return true, nil
	case err != nil:
		return false, errs.Wrap(err, errs.CodeInternal, "social.CommentLiveness.CommentDeleted")
	}
	return ref.Deleted, nil
}
