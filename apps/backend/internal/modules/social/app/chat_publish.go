package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

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
	LastReplyAt time.Time `json:"last_reply_at"`
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
