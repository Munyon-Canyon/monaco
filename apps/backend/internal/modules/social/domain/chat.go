package domain

import (
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const ChatBodyMax = 2000

type ChatBody string

func ParseChatBody(raw string) (ChatBody, error) {
	body := strings.TrimSpace(raw)
	n := utf8.RuneCountInString(body)
	if n < 1 || n > ChatBodyMax || !utf8.ValidString(body) {
		return "", errs.New(errs.CodeChatBodyInvalid, "social.ParseChatBody", slog.Int("scalars", n))
	}
	return ChatBody(body), nil
}

func (b ChatBody) String() string { return string(b) }

type Reply struct {
	Parent        uuid.UUID
	AlsoInChannel bool
}

type ChatParent struct {
	CabalID  ids.CabalID
	ParentID uuid.UUID
	Deleted  bool
}

func ValidateReply(in ids.CabalID, parent ChatParent) error {
	const op = "social.ValidateReply"
	switch {
	case parent.CabalID != in || parent.Deleted:
		return errs.New(errs.CodeChatParentNotFound, op, slog.Bool("deleted", parent.Deleted))
	case parent.ParentID != uuid.Nil:
		return errs.New(errs.CodeChatParentIsReply, op)
	default:
		return nil
	}
}
