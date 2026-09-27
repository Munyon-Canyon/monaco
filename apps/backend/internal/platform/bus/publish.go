package bus

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func (c *Conn) Publish(ctx context.Context, subject string, data []byte, msgID ids.EventID) error {
	msg := &nats.Msg{Subject: c.ns.subject(subject), Data: data, Header: nats.Header{}}
	observability.Inject(ctx, natsCarrier(msg.Header))
	if _, err := c.js.PublishMsg(ctx, msg, jetstream.WithMsgID(msgID.String())); err != nil {
		return errs.Wrap(err, publishCode(err), "bus.Publish",
			slog.String("subject", subject), slog.String("msg_id", msgID.String()))
	}
	return nil
}

func publishCode(err error) errs.Code {
	var apiErr *jetstream.APIError
	if errors.As(err, &apiErr) && slices.Contains(storageFull(), apiErr.ErrorCode) {
		return errs.CodeUpstreamUnavailable
	}
	for _, transient := range []error{
		context.DeadlineExceeded, nats.ErrTimeout, nats.ErrNoResponders, jetstream.ErrNoStreamResponse,
		nats.ErrConnectionClosed, nats.ErrConnectionDraining, nats.ErrConnectionReconnecting,
	} {
		if errors.Is(err, transient) {
			return errs.CodeUpstreamUnavailable
		}
	}
	return errs.CodeInternal
}

func storageFull() []jetstream.ErrorCode {
	const (
		streamStoreFailed     jetstream.ErrorCode = 10077
		insufficientResources jetstream.ErrorCode = 10023
		accountResourcesLimit jetstream.ErrorCode = 10047
	)
	return []jetstream.ErrorCode{streamStoreFailed, insufficientResources, accountResourcesLimit}
}

func (c *Conn) PublishHint(ctx context.Context, key string, payload []byte) {
	if err := c.nc.Publish(c.ns.subject("hint."+key), payload); err != nil {
		c.hintDropped.Add(ctx, 1)
	}
}

func (c *Conn) SubscribeHints(ctx context.Context, fn func(ctx context.Context, key string)) error {
	prefix := c.ns.subject("hint.")
	_, err := c.nc.Subscribe(prefix+">", func(m *nats.Msg) { fn(ctx, strings.TrimPrefix(m.Subject, prefix)) })
	if err == nil {
		err = c.nc.Flush()
	}
	if err != nil {
		return errs.Wrap(err, errs.CodeUpstreamUnavailable, "bus.SubscribeHints")
	}
	return nil
}
