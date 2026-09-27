package bus

import (
	"context"
	"encoding/json"
	"log/slog"
	"runtime/debug"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
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
		for _, h := range handlers {
			results = append(results, r.handle(ctx, h, id, ev))
		}
	}
	r.respond(ctx, durable, msg, results)
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

func (r *Registry) decode(handlers []HandlerSpec, msg jetstream.Msg) (uuid.UUID, events.Event, error) {
	const op = "bus.Dispatch.decode"
	if len(handlers) == 0 {
		return uuid.Nil, nil, errs.New(errs.CodeDecodeFailed, op, slog.String("subject", msg.Subject()))
	}
	raw := msg.Headers().Get(jetstream.MsgIDHeader)
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, nil, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("msg_id", raw))
	}
	var head struct {
		V int `json:"v"`
	}
	if err := json.Unmarshal(msg.Data(), &head); err != nil {
		return uuid.Nil, nil, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("msg_id", raw))
	}
	ev, err := events.Decode(handlers[0].typ, head.V, msg.Data())
	if err != nil {
		return uuid.Nil, nil, err
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

func (r *Registry) handle(ctx context.Context, h HandlerSpec, id uuid.UUID, ev events.Event) result {
	duplicate, err := r.run(ctx, h, id, ev)
	switch {
	case err != nil:
		return failed(h.Name, err)
	case duplicate:
		return result{handler: h.Name, outcome: OutcomeDuplicate, code: deliveryOK}
	default:
		return result{handler: h.Name, outcome: OutcomeAck, code: deliveryOK}
	}
}

func (r *Registry) run(ctx context.Context, h HandlerSpec, id uuid.UUID, ev events.Event) (duplicate bool, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = errs.New(errs.CodePanic, "bus.Dispatch",
				slog.Any("panic", p), slog.String("stack", string(debug.Stack())))
		}
	}()
	err = r.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		inserted, err := sqlc.New(tx.Queries()).InsertDelivery(ctx, sqlc.InsertDeliveryParams{
			Handler: h.Name, EventID: id, Code: deliveryOK, HandledAt: r.clock.Now(),
		})
		if err != nil {
			return err
		}
		if inserted == 0 {
			duplicate = true
			return nil
		}
		return h.run(ctx, tx, ev)
	})
	return duplicate, err
}

func (r *Registry) respond(ctx context.Context, durable string, msg jetstream.Msg, results []result) {
	delivery := numDelivered(msg)
	verdict := OutcomeAck
	for _, res := range results {
		r.log(ctx, durable, msg.Subject(), delivery, res)
		switch {
		case res.outcome == OutcomeNak:
			verdict = OutcomeNak
		case res.outcome == OutcomeTerm && verdict != OutcomeNak:
			verdict = OutcomeTerm
		}
	}
	var err error
	switch verdict {
	case OutcomeNak:
		err = msg.Nak()
	case OutcomeTerm:
		err = msg.Term()
	case OutcomeAck, OutcomeDuplicate:
		err = msg.Ack()
	}
	if err != nil {
		boundary.Error(ctx, observability.BusRespondFailed,
			slog.String("consumer", durable), slog.String("verdict", string(verdict)), slog.Any("err", err))
	}
}

func numDelivered(msg jetstream.Msg) uint64 {
	meta, err := msg.Metadata()
	if err != nil {
		return 0
	}
	return meta.NumDelivered
}

func (r *Registry) log(ctx context.Context, durable, subject string, delivery uint64, res result) {
	switch res.outcome {
	case OutcomeTerm:
		boundary.Error(ctx, observability.BusDispatched,
			slog.String("consumer", durable), slog.String("handler", res.handler), slog.String("subject", subject),
			slog.String("outcome", string(res.outcome)), slog.String("code", res.code),
			slog.Uint64("delivery", delivery),
			slog.Any("err", res.err), slog.Bool("alert", errs.Alert(errs.Code(res.code))))
	case OutcomeNak:
		boundary.Warn(ctx, observability.BusDispatched,
			slog.String("consumer", durable), slog.String("handler", res.handler), slog.String("subject", subject),
			slog.String("outcome", string(res.outcome)), slog.String("code", res.code),
			slog.Uint64("delivery", delivery),
			slog.Any("err", res.err))
	case OutcomeAck, OutcomeDuplicate:
		observability.Info(ctx, observability.BusDispatched,
			slog.String("consumer", durable), slog.String("handler", res.handler), slog.String("subject", subject),
			slog.String("outcome", string(res.outcome)), slog.String("code", res.code),
			slog.Uint64("delivery", delivery))
	}
}
