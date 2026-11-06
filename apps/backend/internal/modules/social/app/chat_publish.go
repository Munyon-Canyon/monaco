package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	EventMessageCreated  = "message.created"
	EventThreadUpdated   = "thread.updated"
	EventMessageDeleted  = "message.deleted"
	EventSeenUpdated     = "seen.updated"
	EventReactionUpdated = "reaction.updated"
	EventPollUpdated     = "poll.updated"
	EventMomentCreated   = "moment.created"
	EventPinUpdated      = "pin.updated"
)

type ChatWire func(ctx context.Context, m ChatMessage) (any, error)

type ThreadUpdated struct {
	MessageID   uuid.UUID `json:"message_id"`
	ReplyCount  int32     `json:"reply_count"`
	LastReplyAt time.Time `json:"last_reply_at,omitzero"`
}

type MessageDeleted struct {
	ID uuid.UUID `json:"id"`
}

type SeenUpdated struct {
	MessageID uuid.UUID `json:"message_id"`
	Count     int       `json:"count"`
}

type ReactionCount struct {
	Emoji string `json:"emoji"`
	Count int32  `json:"count"`
}

type ReactionUpdated struct {
	MessageID uuid.UUID       `json:"message_id"`
	Reactions []ReactionCount `json:"reactions"`
	Total     int32           `json:"total"`
}

type PollOptionCount struct {
	Position int `json:"position"`
	Votes    int `json:"votes"`
}

type PollUpdated struct {
	MessageID  uuid.UUID         `json:"message_id"`
	Options    []PollOptionCount `json:"options"`
	TotalVotes int               `json:"total_votes"`
}

type PinUpdated struct {
	MessageID uuid.UUID `json:"message_id"`
	Pinned    bool      `json:"pinned"`
}

type ChatPublisher struct {
	realtime Realtime
	wire     ChatWire
}

func NewChatPublisher(realtime Realtime, wire ChatWire) ChatPublisher {
	return ChatPublisher{realtime: realtime, wire: wire}
}

func CabalChannel(cabal ids.CabalID) string { return "cabal:" + cabal.String() }

func (p ChatPublisher) created(ctx context.Context, m ChatMessage, thread *ThreadUpdated) {
	ctx = context.WithoutCancel(ctx)
	if payload, err := p.wire(ctx, m); err != nil {
		failedPublish(ctx, m.ID, EventMessageCreated, err)
	} else {
		p.send(ctx, m.CabalID, m.ID, EventMessageCreated, payload)
	}
	if thread != nil {
		p.send(ctx, m.CabalID, m.ID, EventThreadUpdated, thread)
	}
}

func (p ChatPublisher) moment(ctx context.Context, m ChatMessage) {
	ctx = context.WithoutCancel(ctx)
	payload, err := p.wire(ctx, m)
	if err != nil {
		failedPublish(ctx, m.ID, EventMomentCreated, err)
		return
	}
	p.send(ctx, m.CabalID, m.ID, EventMomentCreated, payload)
}

func (p ChatPublisher) deleted(ctx context.Context, cabal ids.CabalID, id uuid.UUID, thread *ThreadUpdated) {
	ctx = context.WithoutCancel(ctx)
	p.send(ctx, cabal, id, EventMessageDeleted, MessageDeleted{ID: id})
	if thread != nil {
		p.send(ctx, cabal, id, EventThreadUpdated, thread)
	}
}

func (p ChatPublisher) reactionUpdated(ctx context.Context, cabal ids.CabalID, id uuid.UUID, s ReactionSummary) {
	ctx = context.WithoutCancel(ctx)
	counts := make([]ReactionCount, len(s.Reactions))
	for i, r := range s.Reactions {
		counts[i] = ReactionCount{Emoji: r.Emoji.String(), Count: r.Count}
	}
	p.send(ctx, cabal, id, EventReactionUpdated, ReactionUpdated{MessageID: id, Reactions: counts, Total: s.Total})
}

func (p ChatPublisher) pollUpdated(ctx context.Context, cabal ids.CabalID, id uuid.UUID, v PollView) {
	ctx = context.WithoutCancel(ctx)
	counts := make([]PollOptionCount, len(v.Options))
	for i, o := range v.Options {
		counts[i] = PollOptionCount{Position: o.Position, Votes: o.Count}
	}
	p.send(ctx, cabal, id, EventPollUpdated, PollUpdated{MessageID: id, Options: counts, TotalVotes: v.Total})
}

func (p ChatPublisher) pinUpdated(ctx context.Context, cabal ids.CabalID, id uuid.UUID, pinned bool) {
	ctx = context.WithoutCancel(ctx)
	p.send(ctx, cabal, id, EventPinUpdated, PinUpdated{MessageID: id, Pinned: pinned})
}

func (p ChatPublisher) send(ctx context.Context, cabal ids.CabalID, id uuid.UUID, event string, data any) {
	if err := p.realtime.Publish(ctx, CabalChannel(cabal), event, data); err != nil {
		failedPublish(ctx, id, event, err)
	}
}

func failedPublish(ctx context.Context, id uuid.UUID, event string, err error) {
	observability.Degraded(ctx, observability.SocialChatPublishFailed,
		slog.String("message_id", id.String()), slog.String("event", event), slog.String("cause", err.Error()))
}

type proposalCard struct {
	id         uuid.UUID
	cabal      ids.CabalID
	proposal   uuid.UUID
	replyCount int32
	lastReply  time.Time
}

func (p ChatPublisher) enabled() bool { return p.realtime != nil && p.wire != nil }

func (p ChatPublisher) cardOf(ctx context.Context, reads sqlc.DBTX, comment uuid.UUID) (proposalCard, bool) {
	row, err := sqlc.New(reads).GetProposalCardOfComment(ctx, comment)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			failedPublish(ctx, comment, EventMessageCreated, err)
		}
		return proposalCard{}, false
	}
	if row.AuthorKind != domain.AuthorKindUser {
		return proposalCard{}, false
	}
	card := proposalCard{id: row.CardID, cabal: ids.CabalIDFrom(row.CabalID), proposal: row.ProposalID}
	if row.ReplyCount > 0 {
		card.replyCount, card.lastReply = row.ReplyCount, row.LastReplyAt.UTC()
	}
	return card, true
}

func (c proposalCard) thread() *ThreadUpdated {
	return &ThreadUpdated{MessageID: c.id, ReplyCount: c.replyCount, LastReplyAt: c.lastReply}
}

func (p ChatPublisher) commentCreated(ctx context.Context, reads sqlc.DBTX, c Comment) {
	if !p.enabled() {
		return
	}
	ctx = context.WithoutCancel(ctx)
	card, ok := p.cardOf(ctx, reads, c.ID)
	if !ok {
		return
	}
	p.created(ctx, chatMessageOfComment(card, c), card.thread())
}

func (p ChatPublisher) commentDeleted(ctx context.Context, reads sqlc.DBTX, comment uuid.UUID) {
	if !p.enabled() {
		return
	}
	ctx = context.WithoutCancel(ctx)
	card, ok := p.cardOf(ctx, reads, comment)
	if !ok {
		return
	}
	p.deleted(ctx, card.cabal, comment, card.thread())
}

func (p ChatPublisher) commentReacted(ctx context.Context, reads sqlc.DBTX, viewer ids.UserID, comment uuid.UUID) {
	if !p.enabled() {
		return
	}
	ctx = context.WithoutCancel(ctx)
	card, ok := p.cardOf(ctx, reads, comment)
	if !ok {
		return
	}
	summary, err := commentReactionSummary(ctx, reads, viewer, comment)
	if err != nil {
		failedPublish(ctx, comment, EventReactionUpdated, err)
		return
	}
	p.reactionUpdated(ctx, card.cabal, comment, summary)
}

func chatMessageOfComment(card proposalCard, c Comment) ChatMessage {
	return ChatMessage{
		ID: c.ID, Kind: domain.ChatKindUser, CabalID: card.cabal, AuthorID: c.AuthorID, Body: c.Body,
		CreatedAt: c.CreatedAt, ParentID: card.id, CommentID: c.ID, Attachment: c.Attachment, Stock: c.Stock,
		Poll: c.Poll, ReplyTo: c.ReplyTo,
	}
}
