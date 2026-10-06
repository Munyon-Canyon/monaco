package domain

import (
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	CommentBodyMax    = 1000
	CommentExcerptMax = 120
)

type CommentBody string

func ParseCommentBody(raw string) (CommentBody, error) {
	body := strings.TrimSpace(raw)
	n := utf8.RuneCountInString(body)
	if n < 1 || n > CommentBodyMax || !utf8.ValidString(body) {
		return "", errs.New(errs.CodeInvalidInput, "social.ParseCommentBody", slog.Int("scalars", n))
	}
	return CommentBody(body), nil
}

func (b CommentBody) String() string { return string(b) }

func (b CommentBody) Excerpt() string {
	runes := []rune(string(b))
	if len(runes) <= CommentExcerptMax {
		return string(b)
	}
	return string(runes[:CommentExcerptMax])
}

type CommentTarget struct {
	ID       uuid.UUID
	AuthorID uuid.UUID
	ParentID uuid.UUID
	Deleted  bool
}

type CommentPlacement struct {
	ParentID  uuid.UUID
	ReplyToID uuid.UUID
}

func PlaceReply(target CommentTarget) CommentPlacement {
	if target.ParentID == uuid.Nil {
		return CommentPlacement{ParentID: target.ID}
	}
	return CommentPlacement{ParentID: target.ParentID, ReplyToID: target.AuthorID}
}
