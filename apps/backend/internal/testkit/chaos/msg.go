package chaos

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Verdict struct {
	Outcome bus.Outcome
	Reason  string
	Delay   time.Duration
}

type Msg struct {
	id        string
	typ       events.Type
	subject   string
	header    nats.Header
	data      []byte
	delivered uint64
	verdict   Verdict
}

var _ jetstream.Msg = (*Msg)(nil)

func NewMsg(conn *bus.Conn, typ events.Type, id ids.EventID, payload []byte) *Msg {
	return &Msg{
		id: id.String(), typ: typ, subject: conn.Subject(typ.Subject()),
		header: nats.Header{jetstream.MsgIDHeader: []string{id.String()}}, data: payload,
	}
}

func (m *Msg) ID() string { return m.id }

func (m *Msg) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{NumDelivered: m.delivered}, nil
}

func (m *Msg) Data() []byte                    { return m.data }
func (m *Msg) Headers() nats.Header            { return m.header }
func (m *Msg) Subject() string                 { return m.subject }
func (m *Msg) Reply() string                   { return "" }
func (m *Msg) Ack() error                      { return m.respond(Verdict{Outcome: bus.OutcomeAck}) }
func (m *Msg) DoubleAck(context.Context) error { return m.Ack() }
func (m *Msg) Nak() error                      { return m.respond(Verdict{Outcome: bus.OutcomeNak}) }

func (m *Msg) NakWithDelay(delay time.Duration) error {
	return m.respond(Verdict{Outcome: bus.OutcomeNak, Delay: delay})
}
func (m *Msg) InProgress() error { return nil }
func (m *Msg) Term() error       { return m.respond(Verdict{Outcome: bus.OutcomeTerm}) }
func (m *Msg) TermWithReason(reason string) error {
	return m.respond(Verdict{Outcome: bus.OutcomeTerm, Reason: reason})
}

func (m *Msg) respond(v Verdict) error {
	m.verdict = v
	return nil
}
