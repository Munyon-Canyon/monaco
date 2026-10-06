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
	EventMessageCreated = "message.created"
	EventThreadUpdated  = "thread.updated"
	EventMessageDeleted = "message.deleted"
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
	payload, err := p.wire(ctx, m)
	if err != nil {
		failedPublish(ctx, m.ID, EventMessageCreated, err)
		return
	}
	p.send(ctx, m.CabalID, m.ID, EventMessageCreated, payload)
	if thread != nil {
		p.send(ctx, m.CabalID, m.ID, EventThreadUpdated, thread)
	}
}

func (p ChatPublisher) deleted(ctx context.Context, cabal ids.CabalID, id uuid.UUID) {
	p.send(context.WithoutCancel(ctx), cabal, id, EventMessageDeleted, MessageDeleted{ID: id})
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
