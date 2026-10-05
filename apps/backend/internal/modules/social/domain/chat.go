package domain

import (
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const MaxChatBodyScalars = 2000

type ReplyParent struct {
	CabalID       uuid.UUID
	ParentCabalID uuid.UUID
	ParentID      uuid.UUID
	DeletedAt     *time.Time
}

func ValidateReply(parent ReplyParent) error {
	const op = "social.ValidateReply"
	switch {
	case parent.ParentID != uuid.Nil:
		return errs.New(errs.CodeInvalidInput, op, slog.String("reason", "reply_to_reply"))
	case parent.CabalID != parent.ParentCabalID:
		return errs.New(errs.CodeInvalidInput, op, slog.String("reason", "other_cabal"))
	case parent.DeletedAt != nil:
		return errs.New(errs.CodeInvalidInput, op, slog.String("reason", "deleted"))
	default:
		return nil
	}
}

func NormalizeBody(raw string) (string, error) {
	const op = "social.NormalizeBody"
	body := strings.TrimSpace(raw)
	count := utf8.RuneCountInString(body)
	if count < 1 || count > MaxChatBodyScalars {
		return "", errs.New(errs.CodeInvalidInput, op, slog.Int("scalars", count))
	}
	return body, nil
}
