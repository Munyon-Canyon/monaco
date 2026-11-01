package testkit

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Delivery struct {
	Delivered uint64
	Outcome   bus.Outcome
	Reason    string
	Delay     time.Duration

	subject string
	header  nats.Header
	data    []byte
}

var _ jetstream.Msg = (*Delivery)(nil)

func NewDelivery(conn *bus.Conn, typ events.Type, id ids.EventID, payload []byte) *Delivery {
	return &Delivery{
		subject: conn.Subject(typ.Subject()),
		header:  nats.Header{jetstream.MsgIDHeader: []string{id.String()}},
		data:    payload,
	}
}

func (m *Delivery) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{NumDelivered: m.Delivered}, nil
}

func (m *Delivery) Data() []byte                    { return m.data }
func (m *Delivery) Headers() nats.Header            { return m.header }
func (m *Delivery) Subject() string                 { return m.subject }
func (m *Delivery) Reply() string                   { return "" }
func (m *Delivery) Ack() error                      { m.Outcome = bus.OutcomeAck; return nil }
func (m *Delivery) DoubleAck(context.Context) error { return m.Ack() }
func (m *Delivery) Nak() error                      { m.Outcome = bus.OutcomeNak; return nil }
func (m *Delivery) InProgress() error               { return nil }
func (m *Delivery) Term() error                     { m.Outcome = bus.OutcomeTerm; return nil }

func (m *Delivery) NakWithDelay(delay time.Duration) error {
	m.Outcome, m.Delay = bus.OutcomeNak, delay
	return nil
}

func (m *Delivery) TermWithReason(reason string) error {
	m.Outcome, m.Reason = bus.OutcomeTerm, reason
	return nil
}
