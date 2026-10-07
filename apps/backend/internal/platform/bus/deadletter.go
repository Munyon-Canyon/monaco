package bus

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	EventIDHeader  = "Monaco-Event-Id"
	RedrivenHeader = "Monaco-Redriven"
	ResolvedCode   = "ok"
)

func EventIDOf(h nats.Header) string {
	if id := h.Get(EventIDHeader); id != "" {
		return id
	}
	return h.Get(jetstream.MsgIDHeader)
}

type messageGetter interface {
	GetMsg(ctx context.Context, seq uint64, opts ...jetstream.GetMsgOpt) (*jetstream.RawStreamMsg, error)
}

func (c *Conn) DeadLetters(ctx context.Context) ([]DeadLetter, error) {
	s, err := c.js.Stream(ctx, c.ns.stream(StreamDeadLetter))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeUpstreamUnavailable, "bus.DeadLetters")
	}
	state := s.CachedInfo().State
	return readDeadLetters(ctx, s, state.FirstSeq, state.LastSeq)
}

func readDeadLetters(ctx context.Context, s messageGetter, first, last uint64) ([]DeadLetter, error) {
	var out []DeadLetter
	for seq := first; seq <= last && seq > 0; seq++ {
		raw, err := s.GetMsg(ctx, seq)
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			continue
		}
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeUpstreamUnavailable, "bus.DeadLetters")
		}
		var letter DeadLetter
		_ = json.Unmarshal(raw.Data, &letter)
		if letter.Code == ResolvedCode {
			continue
		}
		letter.Seq = seq
		out = append(out, letter)
	}
	return out, nil
}

func (c *Conn) EventAt(ctx context.Context, seq uint64) (*nats.Msg, error) {
	const op = "bus.EventAt"
	s, err := c.js.Stream(ctx, c.ns.stream(StreamEvents))
	var raw *jetstream.RawStreamMsg
	if err == nil {
		raw, err = s.GetMsg(ctx, seq)
	}
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeNotFound, op, slog.Uint64("seq", seq))
	}
	return &nats.Msg{Subject: raw.Subject, Data: raw.Data, Header: raw.Header}, nil
}

func (c *Conn) PullDeadLetters(
	ctx context.Context, durable string, limit int, fn func(context.Context, DeadLetter) error,
) (int, error) {
	const op = "bus.PullDeadLetters"
	attrs := []slog.Attr{slog.String("consumer", durable)}
	cons, err := c.js.CreateOrUpdateConsumer(ctx, c.ns.stream(StreamDeadLetter), jetstream.ConsumerConfig{
		Durable:        durable,
		FilterSubjects: []string{c.ns.subject("deadletter.>")},
		DeliverPolicy:  jetstream.DeliverAllPolicy,
		AckPolicy:      jetstream.AckExplicitPolicy,
		AckWait:        30 * time.Second,
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeUpstreamUnavailable, op, attrs...)
	}
	return drain(ctx, cons.Fetch, limit, fn)
}

type fetchFunc func(batch int, opts ...jetstream.FetchOpt) (jetstream.MessageBatch, error)

func drain(ctx context.Context, fetch fetchFunc, limit int, fn func(context.Context, DeadLetter) error) (int, error) {
	const op = "bus.PullDeadLetters"
	batch, err := fetch(limit, jetstream.FetchMaxWait(500*time.Millisecond))
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeUpstreamUnavailable, op)
	}
	handled := 0
	var failure error
	var unhandled []jetstream.Msg
	for msg := range batch.Messages() {
		if failure == nil {
			if failure = pulled(ctx, msg, fn); failure == nil {
				handled++
				continue
			}
		}
		unhandled = append(unhandled, msg)
	}
	for _, msg := range unhandled {
		_ = msg.Nak()
	}
	if failure != nil {
		return handled, failure
	}
	if err := batch.Error(); err != nil {
		return handled, errs.Wrap(err, errs.CodeUpstreamUnavailable, op)
	}
	return handled, nil
}

func pulled(ctx context.Context, msg jetstream.Msg, fn func(context.Context, DeadLetter) error) error {
	var letter DeadLetter
	if err := json.Unmarshal(msg.Data(), &letter); err != nil {
		return answered(msg.Term())
	}
	meta, err := msg.Metadata()
	if err != nil {
		return answered(err)
	}
	letter.Seq = meta.Sequence.Stream
	if err := fn(ctx, letter); err != nil {
		return err
	}
	return answered(msg.Ack())
}

func answered(err error) error {
	if err == nil {
		return nil
	}
	return errs.Wrap(err, errs.CodeUpstreamUnavailable, "bus.PullDeadLetters")
}

func (c *Conn) Redeliver(ctx context.Context, letter DeadLetter, suffix string) error {
	const op = "bus.Redeliver"
	attrs := []slog.Attr{slog.Uint64("seq", letter.Seq)}
	if letter.Code == ResolvedCode {
		return errs.New(errs.CodeNotFound, op, attrs...)
	}
	msg := &nats.Msg{Subject: letter.Subject, Data: letter.Data, Header: letter.Headers}
	if letter.Advisory != nil {
		var advisory struct {
			StreamSeq uint64 `json:"stream_seq"`
		}
		_ = json.Unmarshal(letter.Advisory, &advisory)
		event, err := c.EventAt(ctx, advisory.StreamSeq)
		if err != nil {
			return errs.Wrap(err, errs.CodeNotFound, op, attrs...)
		}
		msg = event
	}
	id := EventIDOf(msg.Header)
	header := nats.Header{}
	for k, v := range msg.Header {
		header[k] = v
	}
	header.Del(jetstream.MsgIDHeader)
	header.Set(EventIDHeader, id)
	header.Set(RedrivenHeader, letter.Consumer)
	msg.Header = header
	if _, err := c.js.PublishMsg(ctx, msg, jetstream.WithMsgID(id+"/"+suffix)); err != nil {
		return errs.Wrap(err, publishCode(err), op, attrs...)
	}
	return nil
}
