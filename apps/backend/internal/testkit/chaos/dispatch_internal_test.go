//go:build faultpoints

package chaos

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

type stopRun struct{}

type fakeTB struct {
	testing.TB
	fatal string
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Fatal(args ...any) { f.fatalf("%s", fmt.Sprint(args...)) }

func (f *fakeTB) Fatalf(format string, args ...any) { f.fatalf(format, args...) }

func (f *fakeTB) fatalf(format string, args ...any) {
	f.fatal = fmt.Sprintf(format, args...)
	panic(stopRun{})
}

func fatalOf(fn func(tb testing.TB)) (fatal string) {
	tb := &fakeTB{}
	defer func() {
		if p := recover(); p != nil && p != (stopRun{}) {
			panic(p)
		}
		fatal = tb.fatal
	}()
	fn(tb)
	return tb.fatal
}

type fakeReg struct {
	verdicts map[string][]Verdict
	first    []string
	calls    map[string]int
	silent   bool
}

func (f *fakeReg) Dispatch(ctx context.Context, _ string, msg jetstream.Msg) {
	m := msg.(*Msg)
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	if f.calls[m.id]++; f.calls[m.id] == 1 {
		f.first = append(f.first, m.id)
	}
	for range 2 {
		faultpoint.Hit(ctx, faultpoint.BeforeCommit)
	}
	if f.silent {
		return
	}
	v := Verdict{Outcome: bus.OutcomeAck}
	if queued := f.verdicts[m.id]; len(queued) > 0 {
		v, f.verdicts[m.id] = queued[0], queued[1:]
	}
	m.verdict = v
}

func twoHandlers() bus.Consumer {
	noop := func(context.Context, db.Tx, events.SystemPinged, time.Time) error { return nil }
	return bus.Consumer{Durable: "d", Handlers: []bus.HandlerSpec{bus.Handle("a", noop), bus.Handle("b", noop)}}
}

func msgs(n int) []*Msg {
	out := make([]*Msg, n)
	for i := range out {
		out[i] = &Msg{id: strconv.Itoa(i), typ: events.TypeSystemPinged}
	}
	return out
}

func chaosRun(tb testing.TB, seed Seed, reg dispatcher, ms []*Msg) Result {
	tb.Helper()
	r := newRun(tb, reg, twoHandlers(), ms)
	r.rng = newRNG(seed)
	r.window = window
	return r.drain(context.Background())
}

func TestDispatch_reordersWithinTheWindowOnly(t *testing.T) {
	t.Parallel()
	reordered := false
	for seed := Seed(1); seed <= 20; seed++ {
		reg := &fakeReg{}
		chaosRun(t, seed, reg, msgs(40))
		for pos, id := range reg.first {
			n, _ := strconv.Atoi(id)
			if overtaken := pos - n; overtaken > window-1 {
				t.Fatalf("seed %d: message %s was delivered after %d earlier ones: %v", seed, id, overtaken, reg.first)
			} else if overtaken > 0 {
				reordered = true
			}
		}
	}
	if !reordered {
		t.Fatal("20 seeds never reordered a message")
	}
}

func TestBaseline_requeuesANakAtNowPlusItsDelayAfterTheRest(t *testing.T) {
	t.Parallel()
	nak := Verdict{Outcome: bus.OutcomeNak, Reason: "upstream_unavailable", Delay: 5 * time.Second}
	reg := &fakeReg{verdicts: map[string][]Verdict{"0": {nak, nak}}}
	r := newRun(t, reg, twoHandlers(), msgs(2))
	r.window = 1
	res := r.drain(context.Background())
	want := []string{
		"t=0s 0 #1 deliver: nak upstream_unavailable -> due 5s",
		"t=0s 1 #1 deliver: ack",
		"t=5s 0 #2 deliver: nak upstream_unavailable -> due 10s",
		"t=10s 0 #3 deliver: ack",
	}
	if !slices.Equal(res.Trace, want) || len(res.Terms()) != 0 {
		t.Fatalf("trace =\n%s\nwant\n%s", strings.Join(res.Trace, "\n"), strings.Join(want, "\n"))
	}
}

func TestBaseline_acksANakkedMessageUpToItsLastDelivery(t *testing.T) {
	t.Parallel()
	nak := Verdict{Outcome: bus.OutcomeNak, Reason: "upstream_unavailable", Delay: time.Second}
	reg := &fakeReg{verdicts: map[string][]Verdict{"0": slices.Repeat([]Verdict{nak}, bus.MaxDeliver-1)}}
	r := newRun(t, reg, twoHandlers(), msgs(1))
	r.window = 1
	res := r.drain(context.Background())
	if v := res.Verdicts["0"]; v.Outcome != bus.OutcomeAck || reg.calls["0"] != bus.MaxDeliver {
		t.Fatalf("verdict %+v after %d deliveries, want ack at delivery %d", v, reg.calls["0"], bus.MaxDeliver)
	}
}

func TestDispatch_aNakAtTheLastDeliveryFailsLoudlyCountingInjectedRedeliveries(t *testing.T) {
	t.Parallel()
	nak := Verdict{Outcome: bus.OutcomeNak, Reason: "upstream_unavailable", Delay: time.Second}
	reg := &fakeReg{verdicts: map[string][]Verdict{"0": slices.Repeat([]Verdict{nak}, bus.MaxDeliver-1)}}
	got := fatalOf(func(tb testing.TB) { tb.Helper(); chaosRun(tb, 3, reg, msgs(1)) })
	want := "chaos: 0 naked at delivery 10, the consumer's MaxDeliver; " +
		"chaos does not simulate the max-deliveries advisory\n"
	if !strings.HasPrefix(got, want) || !strings.Contains(got, "redelivered") {
		t.Fatalf("fatal = %q, want prefix %q after an injected redelivery", got, want)
	}
}

func TestBaseline_failsOnATerm(t *testing.T) {
	t.Parallel()
	reg := &fakeReg{verdicts: map[string][]Verdict{"1": {{Outcome: bus.OutcomeTerm, Reason: "invalid_input"}}}}
	got := fatalOf(func(tb testing.TB) {
		tb.Helper()
		r := newRun(tb, reg, twoHandlers(), msgs(2))
		r.window = 1
		res := r.drain(context.Background())
		if terms := res.Terms(); !slices.Equal(terms, []string{"1 invalid_input"}) {
			tb.Fatalf("terms = %v", terms)
		}
		baselineCheck(tb, res)
	})
	if got != "chaos: the in-order run termed, so the consumer cannot handle its own generated events:\n1 invalid_input" {
		t.Fatalf("fatal = %q", got)
	}
}

func TestDispatch_aDispatchWithoutAVerdictIsAHarnessBug(t *testing.T) {
	t.Parallel()
	got := fatalOf(func(tb testing.TB) { tb.Helper(); chaosRun(tb, 1, &fakeReg{silent: true}, msgs(1)) })
	if !strings.HasPrefix(got, "chaos: dispatch of 0 returned without a verdict") {
		t.Fatalf("fatal = %q", got)
	}
}

func TestDispatch_recoversACrashRedeliversAndIsDeterministic(t *testing.T) {
	t.Parallel()
	first := chaosRun(t, 7, &fakeReg{}, msgs(12))
	again := chaosRun(t, 7, &fakeReg{}, msgs(12))
	if !slices.Equal(first.Trace, again.Trace) {
		t.Fatalf("seed 7 traced\n%s\nthen\n%s", strings.Join(first.Trace, "\n"), strings.Join(again.Trace, "\n"))
	}
	crashes := 0
	for _, line := range first.Trace {
		if strings.Contains(line, "crash-before-commit: crashed before commit ") {
			crashes++
		}
	}
	if crashes == 0 || len(first.Verdicts) != 12 {
		t.Fatalf("%d crashes, %d verdicts; want crashes and every message settled\n%s",
			crashes, len(first.Verdicts), strings.Join(first.Trace, "\n"))
	}
	if other := chaosRun(t, 8, &fakeReg{}, msgs(12)); slices.Equal(other.Trace, first.Trace) {
		t.Fatal("seeds 7 and 8 produced the same trace")
	}
}

type panicky struct{}

func (panicky) Dispatch(context.Context, string, jetstream.Msg) { panic("boom") }

func TestDispatch_repanicsAnythingButACrash(t *testing.T) {
	t.Parallel()
	defer func() {
		if p := recover(); p != "boom" {
			t.Fatalf("recovered %v, want boom", p)
		}
	}()
	chaosRun(t, 1, panicky{}, msgs(1))
}

func TestSeedRange_defaultsToFiftyAndReadsCHAOS_SEEDS(t *testing.T) {
	t.Parallel()
	if got := seedRange(t, ""); len(got) != defaultSeeds || got[0] != 1 || got[len(got)-1] != defaultSeeds {
		t.Fatalf("default seeds = %v, want 1..%d", got, defaultSeeds)
	}
	if got := seedRange(t, "3"); !slices.Equal(got, []Seed{1, 2, 3}) {
		t.Fatalf("CHAOS_SEEDS=3 seeds = %v, want 1..3", got)
	}
	for _, raw := range []string{"0", "many", "-1"} {
		got := fatalOf(func(tb testing.TB) { tb.Helper(); seedRange(tb, raw) })
		if got != fmt.Sprintf("chaos: CHAOS_SEEDS=%q, want a positive integer", raw) {
			t.Fatalf("CHAOS_SEEDS=%s: fatal = %q", raw, got)
		}
	}
}
