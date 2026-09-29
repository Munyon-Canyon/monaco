package bus

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const EventIDHeader = "Monaco-Event-Id"

func eventIDOf(h nats.Header) string {
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
		letter.Seq = seq
		out = append(out, letter)
	}
	return out, nil
}

func (c *Conn) Redeliver(ctx context.Context, letter DeadLetter, suffix string) error {
	const op = "bus.Redeliver"
	attrs := []slog.Attr{slog.Uint64("seq", letter.Seq)}
	msg := &nats.Msg{Subject: letter.Subject, Data: letter.Data, Header: letter.Headers}
	if letter.Advisory != nil {
		var advisory struct {
			StreamSeq uint64 `json:"stream_seq"`
		}
		_ = json.Unmarshal(letter.Advisory, &advisory)
		s, err := c.js.Stream(ctx, c.ns.stream(StreamEvents))
		var raw *jetstream.RawStreamMsg
		if err == nil {
			raw, err = s.GetMsg(ctx, advisory.StreamSeq)
		}
		if err != nil {
			return errs.Wrap(err, errs.CodeNotFound, op, attrs...)
		}
		msg = &nats.Msg{Subject: raw.Subject, Data: raw.Data, Header: raw.Header}
	}
	id := eventIDOf(msg.Header)
	header := nats.Header{}
	for k, v := range msg.Header {
		header[k] = v
	}
	header.Del(jetstream.MsgIDHeader)
	header.Set(EventIDHeader, id)
	msg.Header = header
	if _, err := c.js.PublishMsg(ctx, msg, jetstream.WithMsgID(id+"/"+suffix)); err != nil {
		return errs.Wrap(err, publishCode(err), op, attrs...)
	}
	return nil
}
