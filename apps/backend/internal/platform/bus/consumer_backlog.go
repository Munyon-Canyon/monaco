package bus

import (
	"context"
	"math"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type ConsumerBacklog struct {
	Pending int64
	Oldest  time.Duration
}

func (c *Conn) ConsumerBacklog(ctx context.Context, now time.Time) (ConsumerBacklog, error) {
	const op = "bus.ConsumerBacklog"
	s, err := c.js.Stream(ctx, c.ns.stream(StreamEvents))
	if err != nil {
		return ConsumerBacklog{}, errs.Wrap(err, errs.CodeUpstreamUnavailable, op)
	}
	lister := s.ListConsumers(ctx)
	var out ConsumerBacklog
	for info := range lister.Info() {
		out = out.merge(info, now)
	}
	if err := lister.Err(); err != nil {
		return ConsumerBacklog{}, errs.Wrap(err, errs.CodeUpstreamUnavailable, op)
	}
	return out, nil
}

func (b ConsumerBacklog) merge(info *jetstream.ConsumerInfo, now time.Time) ConsumerBacklog {
	pending := int64(min(info.NumPending, math.MaxInt64)) + int64(info.NumAckPending)
	if pending == 0 {
		return b
	}
	b.Pending += pending
	if last := info.AckFloor.Last; last != nil {
		b.Oldest = max(b.Oldest, now.Sub(*last))
	}
	return b
}
