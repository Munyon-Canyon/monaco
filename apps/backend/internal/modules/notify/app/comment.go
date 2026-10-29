package app

import (
	"context"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	commentExcerptMax = 120
	replyRuleMembers  = "members"
	attachmentGIF     = "gif"
)

type Blocks interface {
	BlockedByMe(ctx context.Context, viewer, user ids.UserID) (bool, error)
}

type CommentReply struct {
	Users  Users
	Cabals Cabals
	Blocks Blocks
}

func (CommentReply) Name() string { return "comment_reply" }

func answeredAuthor(e events.CommentCreated) (ids.UserID, bool) {
	switch {
	case e.ReplyToAuthorID != nil:
		if e.ReplyToDeleted {
			return ids.UserID{}, false
		}
		return ids.UserIDFrom(*e.ReplyToAuthorID), true
	case e.ParentAuthorID != nil && !e.ParentDeleted:
		return ids.UserIDFrom(*e.ParentAuthorID), true
	}
	return ids.UserID{}, false
}

func (k CommentReply) Recipients(ctx context.Context, e events.CommentCreated) ([]ids.UserID, error) {
	to, ok := answeredAuthor(e)
	if !ok {
		return nil, nil
	}
	return unblocked(ctx, k.Blocks, e, to)
}

func unblocked(ctx context.Context, blocks Blocks, e events.CommentCreated, to ids.UserID) ([]ids.UserID, error) {
	replier := ids.UserIDFrom(e.AuthorID)
	for _, pair := range [][2]ids.UserID{{to, replier}, {replier, to}} {
		blocked, err := blocks.BlockedByMe(ctx, pair[0], pair[1])
		if err != nil {
			return nil, err
		}
		if blocked {
			return nil, nil
		}
	}
	return []ids.UserID{to}, nil
}

func collapseID(e events.CommentCreated) string {
	root := e.ParentCommentID
	if e.ReplyToRootID != nil {
		root = e.ReplyToRootID
	}
	return "comment-" + root.String()
}

func (CommentReply) ItemKey(e events.CommentCreated) string {
	return domain.ItemKeyCommentPrefix + e.CommentID.String()
}

func (k CommentReply) Render(ctx context.Context, e events.CommentCreated, to ids.UserID) (Message, error) {
	card, err := replierCard(ctx, k.Users, ids.UserIDFrom(e.AuthorID))
	if err != nil {
		return Message{}, err
	}
	pres, err := replyPresentation(ctx, k.Cabals, e, card)
	if err != nil {
		return Message{}, err
	}
	data := replyData(k.Name(), e)
	if e.ReplyToExcerpt != nil {
		data["quoted_excerpt"] = *e.ReplyToExcerpt
	}
	body := replyBody(e)
	if body != "" {
		body += " "
	}
	if e.ItemActorID != nil && ids.UserIDFrom(*e.ItemActorID) == to {
		body += "On your " + e.ItemLabel + "."
	} else {
		body += "On " + e.ItemActorName + "'s " + e.ItemLabel + "."
	}
	return Message{
		Title:        PersonSender(card).Name + " replied to you",
		Body:         body,
		Data:         data,
		CollapseID:   collapseID(e),
		Presentation: pres,
	}, nil
}

func replierCard(ctx context.Context, users Users, id ids.UserID) (identity.UserCard, error) {
	cards, err := users.UsersByID(ctx, []ids.UserID{id})
	if err != nil {
		return identity.UserCard{}, err
	}
	card := cards[id]
	card.ID = id
	return card, nil
}

func replyPresentation(
	ctx context.Context, cabals Cabals, e events.CommentCreated, replier identity.UserCard,
) (Presentation, error) {
	if e.ReplyRule != replyRuleMembers || e.CabalID == nil {
		return ForPerson(replier), nil
	}
	view, err := cabals.Cabal(ctx, ids.CabalIDFrom(*e.CabalID))
	if err != nil {
		return Presentation{}, err
	}
	return ForCabal(view, PersonSender(replier)), nil
}

func replyBody(e events.CommentCreated) string {
	switch {
	case e.Excerpt != "":
		body := e.Excerpt
		if utf8.RuneCountInString(body) >= commentExcerptMax {
			body += "…"
		}
		return "“" + body + "”"
	case e.PollQuestion != "":
		return "Asked: " + e.PollQuestion
	case e.AttachmentKind == attachmentGIF:
		return "Sent a GIF"
	}
	return ""
}

func replyData(kind string, e events.CommentCreated) map[string]string {
	root := e.CommentID
	if e.ParentCommentID != nil {
		root = *e.ParentCommentID
	}
	data := map[string]string{
		"kind":            kind,
		"feed_item_id":    e.ThreadFeedObjectID.String(),
		"comment_id":      e.CommentID.String(),
		"root_comment_id": root.String(),
		"item_kind":       e.ItemKind,
		"item_label":      e.ItemLabel,
		"item_actor_name": e.ItemActorName,
	}
	optional := map[string]*uuid.UUID{"proposal_id": e.ProposalID, "cabal_id": e.CabalID}
	for key, id := range optional {
		if id != nil {
			data[key] = id.String()
		}
	}
	if e.CabalName != nil {
		data["cabal_name"] = *e.CabalName
	}
	return data
}
