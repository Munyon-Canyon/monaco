package bus

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/nats-io/nats.go"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func (c *Conn) PublishCore(ctx context.Context, m events.Core) error {
	subject := string(m.Type())
	data, _ := json.Marshal(m)
	msg := &nats.Msg{Subject: c.ns.subject(subject), Data: data, Header: nats.Header{}}
	observability.Inject(ctx, natsCarrier(msg.Header))
	if err := c.nc.PublishMsg(msg); err != nil {
		return errs.Wrap(err, errs.CodeUpstreamUnavailable, "bus.PublishCore", slog.String("subject", subject))
	}
	return nil
}
