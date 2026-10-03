package bus

import (
	"context"
	"encoding/json"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

const deliveryOK = "ok"

type Outcome string

const (
	OutcomeAck       Outcome = "ack"
	OutcomeDuplicate Outcome = "duplicate"
	OutcomeNak       Outcome = "nak"
	OutcomeTerm      Outcome = "term"
)

type result struct {
	handler string
	outcome Outcome
	code    string
	err     error
}

func (r *Registry) Dispatch(ctx context.Context, durable string, msg jetstream.Msg) {
	c, ok := r.consumers[durable]
	if !ok {
		panic("bus: Dispatch for unregistered consumer " + durable)
	}
	delivery := numDelivered(msg)
	ctx = observability.Extract(ctx, natsCarrier(msg.Headers()))
	ctx = observability.WithConsumer(ctx, durable, delivery)
	ctx = withKeepAlive(ctx, msg, r.clock)
	handlers := r.route(c, msg.Subject())
	results := make([]result, 0, len(handlers))
	id, ev, err := r.decode(handlers, msg)
	switch {
	case err != nil && len(handlers) == 0:
		results = append(results, failed("", err))
	case err != nil:
		for _, h := range handlers {
			results = append(results, failed(h.Name, err))
		}
	default:
		ctx = observability.WithEventID(ctx, id)
		for _, h := range handlers {
			results = append(results, r.handle(ctx, h, id, ev))
		}
	}
	r.respond(ctx, durable, msg, delivery, results)
}

type keepAliveKey struct{}

type keepAlive struct {
	msg   jetstream.Msg
	clock clock.Clock
}

const keepAliveEvery = 10 * time.Second

func withKeepAlive(ctx context.Context, msg jetstream.Msg, clk clock.Clock) context.Context {
	return context.WithValue(ctx, keepAliveKey{}, keepAlive{msg: msg, clock: clk})
}

func KeepAlive(ctx context.Context) func() {
	k, ok := ctx.Value(keepAliveKey{}).(keepAlive)
	if !ok {
		return func() {}
	}
	ticker := k.clock.NewTicker(keepAliveEvery)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C():
				_ = k.msg.InProgress()
			case <-stop:
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}

func (r *Registry) route(c Consumer, subject string) []HandlerSpec {
	var out []HandlerSpec
	for _, h := range c.Handlers {
		if r.conn.ns.subject(h.typ.Subject()) == subject {
			out = append(out, h)
		}
	}
	return out
}

func (r *Registry) decode(handlers []HandlerSpec, msg jetstream.Msg) (ids.EventID, events.Event, error) {
	const op = "bus.Dispatch.decode"
	var none ids.EventID
	if len(handlers) == 0 {
		return none, nil, errs.New(errs.CodeDecodeFailed, op, slog.String("subject", msg.Subject()))
	}
	raw := eventIDOf(msg.Headers())
	id, err := ids.ParseEventID(raw)
	if err != nil {
		return none, nil, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("msg_id", raw))
	}
	var head struct {
		V int `json:"v"`
	}
	if err := json.Unmarshal(msg.Data(), &head); err != nil {
		return none, nil, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("msg_id", raw))
	}
	ev, err := events.Decode(handlers[0].typ, head.V, msg.Data())
	if err != nil {
		return none, nil, err
	}
	return id, ev, nil
}

func failed(handler string, err error) result {
	code := errs.CodeOf(err)
	res := result{handler: handler, outcome: OutcomeTerm, code: string(code), err: err}
	if errs.VerdictFor(code) == errs.VerdictNak {
		res.outcome = OutcomeNak
	}
	return res
}

func (r *Registry) handle(ctx context.Context, h HandlerSpec, id ids.EventID, ev events.Event) result {
	began := r.clock.Now()
	ctx = observability.WithActor(ctx, "system:"+h.Name)
	duplicate, err := r.run(ctx, h, id, ev)
	res := result{handler: h.Name, outcome: OutcomeAck, code: deliveryOK}
	switch {
	case err != nil:
		res = failed(h.Name, err)
	case duplicate:
		res.outcome = OutcomeDuplicate
	}
	r.durations.Record(ctx, r.clock.Now().Sub(began).Seconds(), metric.WithAttributes(
		attribute.String("consumer", observability.ConsumerFrom(ctx)),
		attribute.String("subject", h.typ.Subject()),
		attribute.String("outcome", string(res.outcome)),
	))
	return res
}

func (r *Registry) run(ctx context.Context, h HandlerSpec, id ids.EventID, ev events.Event) (_ bool, err error) {
	if h.own == nil {
		return Deliver(ctx, r.uow, r.clock, h, id, ev)
	}
	defer recoverPanic(&err)
	return false, h.own(ctx, Delivery{Handler: h.Name, EventID: id, At: r.clock.Now()}, ev)
}

type Delivery struct {
	Handler string
	EventID ids.EventID
	At      time.Time
}

func (d Delivery) Record(ctx context.Context, tx db.Tx) (bool, error) {
	n, err := sqlc.New(tx.Queries()).InsertDelivery(ctx, sqlc.InsertDeliveryParams{
		Handler: d.Handler, EventID: d.EventID.UUID(), Code: deliveryOK, HandledAt: d.At,
	})
	return n == 1, err
}

func Heartbeat(ctx context.Context) func() {
	k, ok := ctx.Value(keepAliveKey{}).(keepAlive)
	if !ok {
		return nil
	}
	return func() { _ = k.msg.InProgress() }
}

func recoverPanic(err *error) {
	if p := recover(); p != nil {
		if faultpoint.IsCrash(p) {
			panic(p)
		}
		*err = errs.New(errs.CodePanic, "bus.Dispatch",
			slog.Any("panic", p), slog.String("stack", string(debug.Stack())))
	}
}

func Deliver(
	ctx context.Context,
	uow *db.UnitOfWork,
	clk clock.Clock,
	h HandlerSpec,
	id ids.EventID,
	ev events.Event,
) (duplicate bool, err error) {
	if h.own != nil {
		return true, nil
	}
	defer recoverPanic(&err)
	fetched, duplicate, err := fetchForDelivery(ctx, uow, h, id, ev)
	if err != nil || duplicate {
		return duplicate, err
	}
	at := clk.Now()
	err = uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		inserted, err := sqlc.New(tx.Queries()).InsertDelivery(ctx, sqlc.InsertDeliveryParams{
			Handler: h.Name, EventID: id.UUID(), Code: deliveryOK, HandledAt: at,
		})
		if err != nil {
			return err
		}
		if inserted == 0 {
			duplicate = true
			return nil
		}
		return h.ApplyFetched(ctx, tx, ev, fetched, at)
	})
	return duplicate, err
}

func fetchForDelivery(
	ctx context.Context,
	uow *db.UnitOfWork,
	h HandlerSpec,
	id ids.EventID,
	ev events.Event,
) (any, bool, error) {
	if h.fetch == nil {
		return noFetchedResult{}, false, nil
	}
	duplicate, err := sqlc.New(uow.Reads()).DeliveryExists(ctx, sqlc.DeliveryExistsParams{
		Handler: h.Name,
		EventID: id.UUID(),
	})
	if err != nil {
		return nil, false, errs.Wrap(err, errs.CodeDBUnavailable, "bus.Deliver")
	}
	if duplicate {
		return nil, true, nil
	}
	fetched, err := h.Fetch(ctx, ev)
	return fetched, false, err
}

type DeadLetter struct {
	Seq      uint64          `json:"-"`
	Consumer string          `json:"consumer"`
	Handler  string          `json:"handler,omitempty"`
	Subject  string          `json:"subject,omitempty"`
	MsgID    string          `json:"msg_id,omitempty"`
	Delivery uint64          `json:"delivery,omitempty"`
	Code     string          `json:"code,omitempty"`
	Error    string          `json:"error,omitempty"`
	Headers  nats.Header     `json:"headers,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
	Advisory json.RawMessage `json:"advisory,omitempty"`
}

func (r *Registry) respond(
	ctx context.Context, durable string, msg jetstream.Msg, delivery uint64, results []result,
) {
	verdict := OutcomeAck
	code := ""
	for _, res := range results {
		r.log(ctx, msg.Subject(), res)
		switch {
		case res.outcome == OutcomeNak:
			verdict, code = OutcomeNak, res.code
		case res.outcome == OutcomeTerm && verdict != OutcomeNak:
			verdict, code = OutcomeTerm, res.code
			r.deadLetter(ctx, durable, DeadLetter{
				Consumer: durable, Handler: res.handler, Subject: msg.Subject(),
				MsgID: msg.Headers().Get(jetstream.MsgIDHeader), Delivery: delivery,
				Code: res.code, Error: res.err.Error(), Headers: msg.Headers(), Data: rawJSON(msg.Data()),
			}, res.handler)
		}
	}
	var err error
	switch verdict {
	case OutcomeNak:
		err = msg.NakWithDelay(r.consumers[durable].nakDelay(delivery))
	case OutcomeTerm:
		err = msg.TermWithReason(code)
	case OutcomeAck, OutcomeDuplicate:
		err = msg.Ack()
	}
	if err != nil {
		boundary.Error(
			ctx,
			observability.BusRespondFailed,
			slog.String("verdict", string(verdict)),
			slog.Any("err", err),
		)
	}
}

func rawJSON(data []byte) json.RawMessage {
	if json.Valid(data) {
		return data
	}
	quoted, _ := json.Marshal(string(data))
	return quoted
}

func (r *Registry) deadLetter(ctx context.Context, consumer string, letter DeadLetter, dedupe string) {
	body, _ := json.Marshal(letter)
	subject := r.conn.ns.subject("deadletter." + consumer)
	msgID := letter.MsgID + "/" + dedupe
	_, err := r.conn.js.PublishMsg(ctx, &nats.Msg{Subject: subject, Data: body}, jetstream.WithMsgID(msgID))
	if err != nil {
		boundary.Error(ctx, observability.BusDeadLetterDropped,
			slog.String("consumer", consumer), slog.String("msg_id", letter.MsgID), slog.Any("err", err))
	}
}

func numDelivered(msg jetstream.Msg) uint64 {
	meta, err := msg.Metadata()
	if err != nil {
		return 0
	}
	return meta.NumDelivered
}

func (r *Registry) log(ctx context.Context, subject string, res result) {
	switch res.outcome {
	case OutcomeTerm:
		boundary.Error(ctx, observability.BusDispatched,
			slog.String("handler", res.handler), slog.String("subject", subject),
			slog.String("outcome", string(res.outcome)), slog.String("code", res.code),
			slog.Any("err", res.err), slog.Bool("alert", errs.Alert(errs.Code(res.code))))
	case OutcomeNak:
		boundary.Warn(ctx, observability.BusDispatched,
			slog.String("handler", res.handler), slog.String("subject", subject),
			slog.String("outcome", string(res.outcome)), slog.String("code", res.code),
			slog.Any("err", res.err))
	case OutcomeAck, OutcomeDuplicate:
		observability.Info(ctx, observability.BusDispatched,
			slog.String("handler", res.handler), slog.String("subject", subject),
			slog.String("outcome", string(res.outcome)), slog.String("code", res.code))
	}
}
