package domain

import (
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/textfilter"
)

const ChatBodyMax = 2000

type ChatBody string

func ParseChatBody(raw string) (ChatBody, error) {
	body := strings.TrimSpace(raw)
	n := utf8.RuneCountInString(body)
	if n < 1 || n > ChatBodyMax || !utf8.ValidString(body) {
		return "", errs.New(errs.CodeChatBodyInvalid, "social.ParseChatBody", slog.Int("scalars", n))
	}
	if err := textfilter.Check("social.ParseChatBody", body); err != nil {
		return "", err
	}
	return ChatBody(body), nil
}

func (b ChatBody) String() string { return string(b) }

type ChatInput struct {
	Body       *ChatBody
	Attachment *Attachment
}

func (c ChatInput) Stored() string {
	if c.Body == nil {
		return ""
	}
	return c.Body.String()
}

func (c ChatInput) Text() ChatBody {
	switch {
	case c.Body != nil:
		return *c.Body
	case c.Attachment != nil:
		return ChatBody(c.Attachment.Display())
	}
	return ""
}

func ParseChatInput(body *string, att *AttachmentInput) (ChatInput, error) {
	const op = "social.ParseChatInput"
	var in ChatInput
	if att != nil {
		if att.Kind == string(AttachmentThesisQuote) && att.ProposalID == uuid.Nil {
			return ChatInput{}, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "no_proposal"))
		}
		parsed, err := parseAttachment(*att)
		if err != nil {
			return ChatInput{}, err
		}
		in.Attachment = &parsed
	}
	if body != nil && (strings.TrimSpace(*body) != "" || att == nil) {
		parsed, err := ParseChatBody(*body)
		if err != nil {
			return ChatInput{}, err
		}
		in.Body = &parsed
	}
	if in.Body == nil && in.Attachment == nil {
		return ChatInput{}, errs.New(errs.CodeChatBodyInvalid, op, slog.Int("scalars", 0))
	}
	return in, nil
}

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
