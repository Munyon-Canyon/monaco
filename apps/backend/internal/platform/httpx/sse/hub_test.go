package sse_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type memberships struct {
	mu      sync.Mutex
	cabals  map[ids.UserID][]ids.CabalID
	err     error
	gate    chan struct{}
	entered chan struct{}
}

func newMemberships() *memberships {
	return &memberships{cabals: map[ids.UserID][]ids.CabalID{}}
}

func (m *memberships) set(user ids.UserID, cabals ...ids.CabalID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cabals[user] = cabals
}

func (m *memberships) holdNextLookup() (entered, release chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gate, m.entered = make(chan struct{}), make(chan struct{})
	return m.entered, m.gate
}

func (m *memberships) CabalIDs(_ context.Context, user ids.UserID) ([]ids.CabalID, error) {
	m.mu.Lock()
	snapshot, err, gate, entered := slices.Clone(m.cabals[user]), m.err, m.gate, m.entered
	m.gate, m.entered = nil, nil
	m.mu.Unlock()
	if gate != nil {
		close(entered)
		<-gate
	}
	return snapshot, err
}

type fixture struct {
	hub     *sse.Hub
	reader  *sdkmetric.ManualReader
	members *memberships
	ids     *testkit.IDs
	stop    func()
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{reader: sdkmetric.NewManualReader(), members: newMemberships(), ids: testkit.NewIDs(477)}
	hub, err := sse.NewHub(f.members, sdkmetric.NewMeterProvider(sdkmetric.WithReader(f.reader)))
	if err != nil {
		t.Fatal(err)
	}
	f.hub = hub
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		hub.Run(ctx)
		close(done)
	}()
	f.stop = sync.OnceFunc(func() {
		cancel()
		<-done
	})
	t.Cleanup(f.stop)
	return f
}

func (f *fixture) user(t *testing.T) ids.UserID {
	t.Helper()
	id, err := ids.ParseUserID(f.ids.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *fixture) cabal(t *testing.T) ids.CabalID {
	t.Helper()
	id, err := ids.ParseCabalID(f.ids.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *fixture) register(t *testing.T, user ids.UserID) *sse.Subscription {
	t.Helper()
	sub, err := f.hub.Register(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: user.String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Close)
	return sub
}

func (f *fixture) deliver(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		f.hub.Deliver(t.Context(), k)
	}
}

func (f *fixture) sum(t *testing.T, name string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := f.reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if s, ok := m.Data.(metricdata.Sum[int64]); ok && m.Name == name {
				for _, p := range s.DataPoints {
					total += p.Value
				}
			}
		}
	}
	return total
}

func (f *fixture) barrier(t *testing.T) {
	t.Helper()
	probe := f.user(t)
	sub := f.register(t, probe)
	f.deliver(t, userHint(probe, "barrier"))
	for next(t, sub).Key == sse.Global {
		continue
	}
	sub.Close()
}

func next(t *testing.T, sub *sse.Subscription) sse.Hint {
	t.Helper()
	select {
	case h, ok := <-sub.Hints():
		if !ok {
			t.Fatal("subscription closed, want a hint")
		}
		return h
	case <-time.After(5 * time.Second):
		t.Fatal("no hint within 5s")
	}
	return sse.Hint{}
}

func nothingBuffered(t *testing.T, sub *sse.Subscription) {
	t.Helper()
	select {
	case h := <-sub.Hints():
		t.Fatalf("unexpected hint %+v", h)
	default:
	}
}

func closed(t *testing.T, sub *sse.Subscription) {
	t.Helper()
	select {
	case h, ok := <-sub.Hints():
		if ok {
			t.Fatalf("hint %+v, want a closed subscription", h)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("subscription still open after 5s")
	}
}

func cabalHint(c ids.CabalID) string { return "cabal." + c.String() + ".updated" }

func userHint(u ids.UserID, what string) string { return "user." + u.String() + "." + what }

func TestHub_RegisterScopesHintsToMembership(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	phone, other := f.user(t), f.user(t)
	seven, fortyTwo := f.cabal(t), f.cabal(t)
	f.members.set(phone, seven)
	sub := f.register(t, phone)

	f.deliver(t, cabalHint(fortyTwo), userHint(other, "updated"),
		cabalHint(seven), userHint(phone, "balance"), "global.feed")

	want := []sse.Hint{
		{Key: sse.CabalKey(seven), What: "updated"},
		{Key: sse.UserKey(phone), What: "balance"},
		{Key: sse.Global, What: "feed"},
	}
	for _, w := range want {
		if got := next(t, sub); got != w {
			t.Fatalf("hint = %+v, want %+v; cabal 42 and the other user's hints come first if they leaked", got, w)
		}
	}
	nothingBuffered(t, sub)
	if n := f.sum(t, "monaco_sse_hints_total"); n != 5 {
		t.Fatalf("monaco_sse_hints_total = %d, want 5", n)
	}
}

func TestHubRegister_aPhoneInCabalADoesNotReceiveCabalBHints(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	phone := f.user(t)
	a, b := f.cabal(t), f.cabal(t)
	f.members.set(phone, a)
	sub := f.register(t, phone)

	f.deliver(t, cabalHint(b), "cabal."+b.String()+".proposal_opened", cabalHint(a))

	if got, want := next(t, sub), (sse.Hint{Key: sse.CabalKey(a), What: "updated"}); got != want {
		t.Fatalf("hint = %+v, want %+v; cabal B's hints come first if they leaked", got, want)
	}
	nothingBuffered(t, sub)
}

func TestHub_KeysSpellTheRFCForm(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	u, c := f.user(t), f.cabal(t)
	if got := sse.UserKey(u); string(got) != "user:"+u.String() {
		t.Fatalf("UserKey = %q", got)
	}
	if got := sse.CabalKey(c); string(got) != "cabal:"+c.String() {
		t.Fatalf("CabalKey = %q", got)
	}
	if sse.Global != "global" {
		t.Fatalf("Global = %q", sse.Global)
	}
}

func TestHub_SlowSubscriberDropsWithoutBlockingOthers(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	slow, fast := f.register(t, f.user(t)), f.register(t, f.user(t))
	const sent = 20
	for i := range sent {
		f.deliver(t, "global.feed")
		if got := next(t, fast); got.Key != sse.Global {
			t.Fatalf("fast hint %d = %+v", i, got)
		}
	}
	f.barrier(t)

	if n := len(slow.Hints()); n != sse.Buffer {
		t.Fatalf("slow subscriber holds %d hints, want its buffer of %d", n, sse.Buffer)
	}
	if n := f.sum(t, "monaco_sse_dropped_total"); n != sent-sse.Buffer {
		t.Fatalf("monaco_sse_dropped_total = %d, want %d", n, sent-sse.Buffer)
	}
}

func TestHub_CloseFreesTheSubscriber(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	sub := f.register(t, f.user(t))
	if n := f.sum(t, "monaco_sse_connections"); n != 1 {
		t.Fatalf("connections = %d, want 1", n)
	}

	sub.Close()
	sub.Close()

	closed(t, sub)
	if n := f.sum(t, "monaco_sse_connections"); n != 0 {
		t.Fatalf("connections after Close = %d, want 0", n)
	}
	f.deliver(t, "global.feed")
	f.barrier(t)
	if n := f.sum(t, "monaco_sse_dropped_total"); n != 0 {
		t.Fatalf("a closed subscriber was still routed: dropped = %d", n)
	}
}

func TestHub_ClosingOneConnectionKeepsTheUsersOtherConnectionRouted(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	phone := f.user(t)
	first, second := f.register(t, phone), f.register(t, phone)

	first.Close()
	f.deliver(t, userHint(phone, "balance"), "global.feed")

	for _, w := range []sse.Hint{{Key: sse.UserKey(phone), What: "balance"}, {Key: sse.Global, What: "feed"}} {
		if got := next(t, second); got != w {
			t.Fatalf("hint = %+v, want %+v: closing the other connection unrouted this one", got, w)
		}
	}
}

func TestHub_RegisterInFlightSurvivesTheUsersLastConnectionClosing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	phone := f.user(t)
	seven, fortyTwo := f.cabal(t), f.cabal(t)
	f.members.set(phone, seven)
	first := f.register(t, phone)
	entered, release := f.members.holdNextLookup()
	registered := make(chan *sse.Subscription, 1)
	go func() {
		sub, err := f.hub.Register(context.Background(), auth.Actor{Kind: auth.ActorUser, ID: phone.String()})
		if err != nil {
			t.Error(err)
		}
		registered <- sub
	}()
	<-entered

	first.Close()
	f.members.set(phone, fortyTwo)
	if err := f.hub.Reregister(t.Context(), phone); err != nil {
		t.Fatal(err)
	}
	close(release)
	sub := <-registered
	t.Cleanup(sub.Close)

	f.deliver(t, cabalHint(seven), cabalHint(fortyTwo))
	if got := next(t, sub); got.Key != sse.CabalKey(fortyTwo) {
		t.Fatalf("hint = %+v; the reregister was lost while the register was in flight", got)
	}
}

func TestHub_ReregisterRescopesOpenConnections(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	phone := f.user(t)
	seven, fortyTwo := f.cabal(t), f.cabal(t)
	f.members.set(phone, seven)
	first, second := f.register(t, phone), f.register(t, phone)

	f.members.set(phone, fortyTwo)
	if err := f.hub.Reregister(t.Context(), phone); err != nil {
		t.Fatal(err)
	}
	f.deliver(t, cabalHint(seven), cabalHint(fortyTwo))

	for _, sub := range []*sse.Subscription{first, second} {
		if got := next(t, sub); got.Key != sse.CabalKey(fortyTwo) {
			t.Fatalf("hint = %+v, want cabal 42 only after leaving cabal 7", got)
		}
		nothingBuffered(t, sub)
	}
}

func TestHub_ReregisterWithoutConnectionsChangesNothing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	phone, seven := f.user(t), f.cabal(t)
	if err := f.hub.Reregister(t.Context(), phone); err != nil {
		t.Fatal(err)
	}
	f.members.set(phone, seven)
	sub := f.register(t, phone)
	f.deliver(t, cabalHint(seven))
	if got := next(t, sub); got.Key != sse.CabalKey(seven) {
		t.Fatalf("hint = %+v, want cabal 7", got)
	}
}

func TestHub_RegisterRacingReregisterKeepsTheNewerMembership(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	phone := f.user(t)
	seven, fortyTwo := f.cabal(t), f.cabal(t)
	f.members.set(phone, seven)
	entered, release := f.members.holdNextLookup()
	registered := make(chan *sse.Subscription, 1)
	go func() {
		sub, err := f.hub.Register(context.Background(), auth.Actor{Kind: auth.ActorUser, ID: phone.String()})
		if err != nil {
			t.Error(err)
		}
		registered <- sub
	}()
	<-entered

	f.members.set(phone, fortyTwo)
	if err := f.hub.Reregister(t.Context(), phone); err != nil {
		t.Fatal(err)
	}
	close(release)
	sub := <-registered
	t.Cleanup(sub.Close)

	f.deliver(t, cabalHint(seven), cabalHint(fortyTwo))
	if got := next(t, sub); got.Key != sse.CabalKey(fortyTwo) {
		t.Fatalf("hint = %+v; the stale cabal 7 lookup overwrote the newer membership", got)
	}
	nothingBuffered(t, sub)
}

func TestHub_StaleReregisterDoesNotOverwriteANewerRegister(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	phone := f.user(t)
	seven, fortyTwo := f.cabal(t), f.cabal(t)
	f.members.set(phone, seven)
	first := f.register(t, phone)
	entered, release := f.members.holdNextLookup()
	reregistered := make(chan error, 1)
	go func() { reregistered <- f.hub.Reregister(context.Background(), phone) }()
	<-entered

	f.members.set(phone, fortyTwo)
	second := f.register(t, phone)
	close(release)
	if err := <-reregistered; err != nil {
		t.Fatal(err)
	}

	f.deliver(t, cabalHint(seven), cabalHint(fortyTwo))
	for _, sub := range []*sse.Subscription{first, second} {
		if got := next(t, sub); got.Key != sse.CabalKey(fortyTwo) {
			t.Fatalf("hint = %+v; the stale reregister restored cabal 7", got)
		}
		nothingBuffered(t, sub)
	}
}

func TestHub_RegisterRejectsActorsThatAreNotUsers(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cases := []struct {
		actor auth.Actor
		want  errs.Code
	}{
		{auth.Actor{Kind: auth.ActorAgent, ID: f.user(t).String()}, errs.CodeForbidden},
		{auth.Actor{Kind: auth.ActorUser, ID: "not-a-uuid"}, errs.CodeUnauthorized},
	}
	for _, c := range cases {
		if _, err := f.hub.Register(t.Context(), c.actor); errs.CodeOf(err) != c.want {
			t.Fatalf("Register(%+v) = %v, want %s", c.actor, err, c.want)
		}
	}
	if n := f.sum(t, "monaco_sse_connections"); n != 0 {
		t.Fatalf("connections = %d, want 0", n)
	}
}

func TestHub_MembershipLookupFailuresReachTheCaller(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.members.err = errs.New(errs.CodeDBUnavailable, "test.CabalIDs")
	phone := f.user(t)
	if _, err := f.hub.Register(
		t.Context(),
		auth.Actor{Kind: auth.ActorUser, ID: phone.String()},
	); errs.CodeOf(
		err,
	) != errs.CodeDBUnavailable {
		t.Fatalf("Register = %v, want db_unavailable", err)
	}
	if err := f.hub.Reregister(t.Context(), phone); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Reregister = %v, want db_unavailable", err)
	}
	if n := f.sum(t, "monaco_sse_connections"); n != 0 {
		t.Fatalf("connections = %d, want 0", n)
	}
}

func TestHub_StoppedHubClosesSubscriptionsAndRefusesRegistration(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	phone := f.user(t)
	sub := f.register(t, phone)
	entered, release := f.members.holdNextLookup()
	late := make(chan error, 1)
	go func() {
		_, err := f.hub.Register(context.Background(), auth.Actor{Kind: auth.ActorUser, ID: phone.String()})
		late <- err
	}()
	<-entered

	f.stop()
	close(release)

	closed(t, sub)
	if err := <-late; errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Register in flight at stop = %v, want upstream_unavailable", err)
	}
	if _, err := f.hub.Register(
		t.Context(),
		auth.Actor{Kind: auth.ActorUser, ID: phone.String()},
	); errs.CodeOf(
		err,
	) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Register after stop = %v, want upstream_unavailable", err)
	}
	if err := f.hub.Reregister(t.Context(), phone); err != nil {
		t.Fatalf("Reregister after stop = %v, want nil", err)
	}
	sub.Close()
	f.deliver(t, "global.feed")
	if n := f.sum(t, "monaco_sse_connections"); n != 0 {
		t.Fatalf("connections after stop = %d, want 0", n)
	}
}

func TestHub_DeliverCountsAndDropsUnknownSubjects(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	u, c := f.user(t), f.cabal(t)
	unknown := []string{
		"global", "global.feed.extra", "user." + u.String(), "cabal." + c.String(), "cabal.42.updated",
		"user.not-a-uuid.updated", "trade." + c.String() + ".updated", "cabal." + c.String() + ".a.b",
		"cabal." + c.String() + ".", "", "global.Feed", `global.a"b`,
	}
	sub := f.register(t, u)
	f.deliver(t, unknown...)
	f.deliver(t, "global.price_moved")

	if got := next(t, sub); got != (sse.Hint{Key: sse.Global, What: "price_moved"}) {
		t.Fatalf("hint = %+v, want only global.feed", got)
	}
	if n := f.sum(t, "monaco_sse_unknown_subjects_total"); n != int64(len(unknown)) {
		t.Fatalf("monaco_sse_unknown_subjects_total = %d, want %d", n, len(unknown))
	}
	if n := f.sum(t, "monaco_sse_hints_total"); n != 1 {
		t.Fatalf("monaco_sse_hints_total = %d, want 1", n)
	}
}

func TestHub_DeliverDropsWhenTheHubIsBehind(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	hub, err := sse.NewHub(newMemberships(), sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{hub: hub, reader: reader}
	const sent = 1000
	for range sent {
		hub.Deliver(t.Context(), "global.feed")
	}
	if n := f.sum(t, "monaco_sse_dropped_total"); n <= 0 || n >= sent {
		t.Fatalf(
			"monaco_sse_dropped_total = %d of %d with no hub running, want the inbox to hold some and drop the rest",
			n,
			sent,
		)
	}
}

type failingMeters struct{ noop.MeterProvider }

func (failingMeters) Meter(string, ...metric.MeterOption) metric.Meter { return failingMeter{} }

type failingMeter struct{ noop.Meter }

func (failingMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, errs.New(errs.CodeInternal, "test.Int64Counter")
}

func TestNewHub_failsWhenAnInstrumentCannotBeCreated(t *testing.T) {
	t.Parallel()
	if _, err := sse.NewHub(newMemberships(), failingMeters{}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("NewHub = %v, want internal", err)
	}
}

func TestNoMemberships_reportsNoCabals(t *testing.T) {
	t.Parallel()
	got, err := sse.NoMemberships{}.CabalIDs(t.Context(), newFixture(t).user(t))
	if err != nil || len(got) != 0 {
		t.Fatalf("CabalIDs = %v, %v; want none", got, err)
	}
}

func TestHub_OneNATSSubscriptionServesEveryConnection(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	b := testkit.NATS(t)
	if err := b.Conn.SubscribeHints(t.Context(), f.hub.Deliver); err != nil {
		t.Fatal(err)
	}
	subs := make([]*sse.Subscription, 100)
	for i := range subs {
		subs[i] = f.register(t, f.user(t))
	}

	if n := testkit.NATSSubscriptions(t, b.Conn.Subject("hint.>")); n != 1 {
		t.Fatalf("NATS subscriptions on hint.> with 100 connections = %d, want 1", n)
	}
	b.Conn.PublishHint(t.Context(), "global.leaderboard", nil)
	for _, sub := range subs {
		if got := next(t, sub); got != (sse.Hint{Key: sse.Global, What: "leaderboard"}) {
			t.Fatalf("hint = %+v", got)
		}
	}
}
