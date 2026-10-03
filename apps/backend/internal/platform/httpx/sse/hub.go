package sse

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	Buffer = 16
	inbox  = 256
)

type Key string

const Global Key = "global"

const MembershipChanged = "cabal_access"

func UserKey(id ids.UserID) Key { return Key("user:" + id.String()) }

func CabalKey(id ids.CabalID) Key { return Key("cabal:" + id.String()) }

type Hint struct {
	Key  Key    `json:"key"`
	What string `json:"what"`
}

type MembershipPort interface {
	CabalIDs(ctx context.Context, user ids.UserID) ([]ids.CabalID, error)
}

type NoMemberships struct{}

func (NoMemberships) CabalIDs(context.Context, ids.UserID) ([]ids.CabalID, error) { return nil, nil }

type Hub struct {
	members MembershipPort
	hints   chan Hint
	ops     chan func(context.Context, *state)
	stopped chan struct{}
	metrics metrics
}

type metrics struct {
	connections metric.Int64UpDownCounter
	hints       metric.Int64Counter
	dropped     metric.Int64Counter
	unknown     metric.Int64Counter
	rescopes    metric.Int64Counter
}

func NewHub(members MembershipPort, meters metric.MeterProvider) (*Hub, error) {
	meter := meters.Meter("github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse")
	connections, errConn := meter.Int64UpDownCounter("monaco_sse_connections",
		metric.WithDescription("Open /v1/stream connections."))
	hints, errHints := meter.Int64Counter("monaco_sse_hints_total",
		metric.WithDescription("Hints received from hint.> with a known subject."))
	dropped, errDropped := meter.Int64Counter("monaco_sse_dropped_total",
		metric.WithDescription("Hints dropped because the hub or a subscriber buffer was full."))
	unknown, errUnknown := meter.Int64Counter("monaco_sse_unknown_subjects_total",
		metric.WithDescription("Hints dropped because their subject names no hub key."))
	rescopes, errRescopes := meter.Int64Counter("monaco_sse_rescope_failures_total",
		metric.WithDescription("Membership hints whose user could not be rescoped to their new cabals."))
	if err := errors.Join(errConn, errHints, errDropped, errUnknown, errRescopes); err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "sse.NewHub")
	}
	return &Hub{
		members: members,
		hints:   make(chan Hint, inbox),
		ops:     make(chan func(context.Context, *state)),
		stopped: make(chan struct{}),
		metrics: metrics{
			connections: connections, hints: hints, dropped: dropped, unknown: unknown, rescopes: rescopes,
		},
	}, nil
}

func (h *Hub) Run(ctx context.Context) {
	st := &state{m: h.metrics, scopes: map[ids.UserID]*scope{}, routes: map[Key]map[*Subscription]struct{}{}}
	defer st.stop(context.WithoutCancel(ctx))
	defer close(h.stopped)
	for {
		select {
		case <-ctx.Done():
			return
		case hint := <-h.hints:
			st.route(ctx, hint)
		case op := <-h.ops:
			op(ctx, st)
		}
	}
}

func (h *Hub) Deliver(ctx context.Context, subject string) {
	hint, ok := parse(subject)
	if !ok {
		h.metrics.unknown.Add(ctx, 1)
		return
	}
	h.metrics.hints.Add(ctx, 1)
	if user, ok := membershipChange(subject); ok && h.Reregister(ctx, user) != nil {
		h.metrics.rescopes.Add(ctx, 1)
	}
	select {
	case h.hints <- hint:
	default:
		h.metrics.dropped.Add(ctx, 1)
	}
}

func parse(subject string) (Hint, bool) {
	tokens := strings.Split(subject, ".")
	what := tokens[len(tokens)-1]
	if what == "" || strings.Trim(what, "abcdefghijklmnopqrstuvwxyz0123456789_") != "" {
		return Hint{}, false
	}
	switch {
	case len(tokens) == 2 && tokens[0] == string(Global):
		return Hint{Key: Global, What: what}, true
	case len(tokens) == 3 && tokens[0] == "user":
		id, err := ids.ParseUserID(tokens[1])
		return Hint{Key: UserKey(id), What: what}, err == nil
	case len(tokens) == 3 && tokens[0] == "cabal":
		id, err := ids.ParseCabalID(tokens[1])
		return Hint{Key: CabalKey(id), What: what}, err == nil
	}
	return Hint{}, false
}

func membershipChange(subject string) (ids.UserID, bool) {
	tokens := strings.Split(subject, ".")
	if len(tokens) != 3 || tokens[0] != "user" || tokens[2] != MembershipChanged {
		return ids.UserID{}, false
	}
	user, err := ids.ParseUserID(tokens[1])
	return user, err == nil
}

func (h *Hub) do(op func(context.Context, *state)) bool {
	done := make(chan struct{})
	select {
	case h.ops <- func(ctx context.Context, st *state) { op(ctx, st); close(done) }:
		<-done
		return true
	case <-h.stopped:
		return false
	}
}

type Subscription struct {
	hub      *Hub
	user     ids.UserID
	hints    chan Hint
	attached bool
}

func (s *Subscription) Hints() <-chan Hint { return s.hints }

func (s *Subscription) Close() { s.hub.do(func(ctx context.Context, st *state) { st.remove(ctx, s) }) }

func (h *Hub) Register(ctx context.Context, actor auth.Actor) (*Subscription, error) {
	const op = "sse.Hub.Register"
	if actor.Kind != auth.ActorUser {
		return nil, errs.New(errs.CodeForbidden, op, slog.String("actor_kind", string(actor.Kind)))
	}
	user, err := ids.ParseUserID(actor.ID)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeUnauthorized, op)
	}
	var ticket uint64
	h.do(func(_ context.Context, st *state) { ticket = st.begin(user) })
	keys, err := h.keys(ctx, user)
	if err != nil {
		h.do(func(_ context.Context, st *state) { st.abort(user) })
		return nil, err
	}
	sub := &Subscription{hub: h, user: user, hints: make(chan Hint, Buffer)}
	if !h.do(func(ctx context.Context, st *state) { st.attach(ctx, sub, keys, ticket) }) {
		return nil, errs.New(errs.CodeUpstreamUnavailable, op, slog.String("reason", "hub_stopped"))
	}
	return sub, nil
}

func (h *Hub) Reregister(ctx context.Context, user ids.UserID) error {
	var ticket uint64
	h.do(func(_ context.Context, st *state) { ticket = st.nextTicket() })
	keys, err := h.keys(ctx, user)
	if err != nil {
		return err
	}
	h.do(func(_ context.Context, st *state) { st.rescope(user, keys, ticket) })
	return nil
}

func (h *Hub) keys(ctx context.Context, user ids.UserID) ([]Key, error) {
	cabals, err := h.members.CabalIDs(ctx, user)
	if err != nil {
		return nil, err
	}
	keys := []Key{UserKey(user), Global}
	for _, c := range cabals {
		keys = append(keys, CabalKey(c))
	}
	return keys, nil
}

type state struct {
	m      metrics
	ticket uint64
	scopes map[ids.UserID]*scope
	routes map[Key]map[*Subscription]struct{}
}

type scope struct {
	ticket  uint64
	keys    []Key
	pending int
	subs    map[*Subscription]struct{}
}

func (st *state) nextTicket() uint64 {
	st.ticket++
	return st.ticket
}

func (st *state) begin(user ids.UserID) uint64 {
	sc, ok := st.scopes[user]
	if !ok {
		sc = &scope{subs: map[*Subscription]struct{}{}}
		st.scopes[user] = sc
	}
	sc.pending++
	return st.nextTicket()
}

func (st *state) abort(user ids.UserID) {
	sc := st.scopes[user]
	sc.pending--
	st.forget(user, sc)
}

func (st *state) attach(ctx context.Context, sub *Subscription, keys []Key, ticket uint64) {
	sc := st.scopes[sub.user]
	sc.pending--
	st.apply(sc, keys, ticket)
	sc.subs[sub] = struct{}{}
	sub.attached = true
	st.link(sub, sc.keys)
	st.m.connections.Add(ctx, 1)
}

func (st *state) rescope(user ids.UserID, keys []Key, ticket uint64) {
	if sc, ok := st.scopes[user]; ok {
		st.apply(sc, keys, ticket)
	}
}

func (st *state) apply(sc *scope, keys []Key, ticket uint64) {
	if ticket <= sc.ticket {
		return
	}
	for sub := range sc.subs {
		st.unlink(sub, sc.keys)
		st.link(sub, keys)
	}
	sc.ticket, sc.keys = ticket, keys
}

func (st *state) remove(ctx context.Context, sub *Subscription) {
	if !sub.attached {
		return
	}
	sc := st.scopes[sub.user]
	st.unlink(sub, sc.keys)
	delete(sc.subs, sub)
	sub.attached = false
	close(sub.hints)
	st.m.connections.Add(ctx, -1)
	st.forget(sub.user, sc)
}

func (st *state) forget(user ids.UserID, sc *scope) {
	if sc.pending == 0 && len(sc.subs) == 0 {
		delete(st.scopes, user)
	}
}

func (st *state) link(sub *Subscription, keys []Key) {
	for _, k := range keys {
		subs, ok := st.routes[k]
		if !ok {
			subs = map[*Subscription]struct{}{}
			st.routes[k] = subs
		}
		subs[sub] = struct{}{}
	}
}

func (st *state) unlink(sub *Subscription, keys []Key) {
	for _, k := range keys {
		delete(st.routes[k], sub)
		if len(st.routes[k]) == 0 {
			delete(st.routes, k)
		}
	}
}

func (st *state) route(ctx context.Context, hint Hint) {
	for sub := range st.routes[hint.Key] {
		select {
		case sub.hints <- hint:
		default:
			st.m.dropped.Add(ctx, 1)
		}
	}
}

func (st *state) stop(ctx context.Context) {
	for _, sc := range st.scopes {
		for sub := range sc.subs {
			close(sub.hints)
			st.m.connections.Add(ctx, -1)
		}
	}
}
