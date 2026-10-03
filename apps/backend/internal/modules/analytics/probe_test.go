package analytics_test

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/adapters/testdata"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestAnalytics_Probe_ExportsWithEventIDAsUUID(t *testing.T) {
	t.Parallel()
	e := newEnv(t, testdata.Register)
	user := e.ids.NewV7()
	a := e.append(t, userActor(user))
	m := e.message(a)
	e.deliver(t, m)
	captures := e.fake.Captures()
	if m.outcome != bus.OutcomeAck || len(captures) != 1 {
		t.Fatalf("verdict %q with %d captures, want ack and one: %+v", m.outcome, len(captures), captures)
	}
	sameCapture(t, captures[0], fakes.PostHogCapture{
		APIKey: apiKey, UUID: a.id, Event: testdata.ProbeEvent, DistinctID: user.String(), Timestamp: a.created,
		Properties: map[string]any{"ping_id": a.ping.PingID.String()},
	})
	sent := logged(t, e, "analytics.capture_sent")
	if len(sent) != 1 || sent[0]["uuid"] != a.id.String() || sent[0]["event"] != testdata.ProbeEvent {
		t.Fatalf("capture_sent lines = %v, want one for %s %s", sent, testdata.ProbeEvent, a.id)
	}
}

func TestAnalytics_Probe_RedeliveryDoesNotResend(t *testing.T) {
	t.Parallel()
	e := newEnv(t, testdata.Register)
	a := e.append(t, userActor(e.ids.NewV7()))
	first, duplicate := e.message(a), e.message(a)
	e.deliver(t, first)
	e.deliver(t, duplicate)
	if first.outcome != bus.OutcomeAck || duplicate.outcome != bus.OutcomeAck || e.fake.Received() != 1 ||
		e.deliveries(t) != 1 {
		t.Fatalf("verdicts %q and %q with %d received and %d delivery rows, want two acks, one received, one row",
			first.outcome, duplicate.outcome, e.fake.Received(), e.deliveries(t))
	}
}

func TestAnalytics_PostHog503_NaksAndRecovers(t *testing.T) {
	t.Parallel()
	e := newEnv(t, testdata.Register)
	e.fake.Fail(t, http.StatusServiceUnavailable, 2)
	a := e.append(t, userActor(e.ids.NewV7()))
	m := e.message(a)
	type attempt struct {
		outcome bus.Outcome
		delay   time.Duration
	}
	got := make([]attempt, 0, 3)
	for range 3 {
		e.deliver(t, m)
		got = append(got, attempt{m.outcome, m.delay})
	}
	schedule := bus.NakSchedule()
	want := []attempt{{bus.OutcomeNak, schedule[0]}, {bus.OutcomeNak, schedule[1]}, {bus.OutcomeAck, 0}}
	if !reflect.DeepEqual(got, want) || len(e.fake.Captures()) != 1 || len(e.deadLetters(t)) != 0 ||
		e.deliveries(t) != 1 {
		t.Fatalf("attempts %+v with %d captures, %d dead letters, %d delivery rows; want %+v, one, none, one",
			got, len(e.fake.Captures()), len(e.deadLetters(t)), e.deliveries(t), want)
	}
	failed := logged(t, e, "analytics.capture_failed")
	if len(failed) != 2 || failed[0]["code"] != string(errs.CodePostHogUnavailable) ||
		failed[0]["uuid"] != a.id.String() {
		t.Fatalf("capture_failed lines = %v, want two post_hog_unavailable for %s", failed, a.id)
	}
}

func TestAnalytics_PostHog400_DeadLetters(t *testing.T) {
	t.Parallel()
	e := newEnv(t, testdata.Register)
	e.fake.Fail(t, http.StatusBadRequest, 1)
	a := e.append(t, userActor(e.ids.NewV7()))
	m := e.message(a)
	e.deliver(t, m)
	letters := e.deadLetters(t)
	rejected := string(errs.CodePostHogRejected)
	if m.outcome != bus.OutcomeTerm || m.reason != rejected || len(letters) != 1 || letters[0].Code != rejected ||
		letters[0].Consumer != "analytics" || letters[0].Handler != pingHandler || e.fake.Received() != 0 ||
		e.deliveries(t) != 0 {
		t.Fatalf("verdict %q %q with dead letters %+v, %d received, %d delivery rows; want term %s and one letter",
			m.outcome, m.reason, letters, e.fake.Received(), e.deliveries(t), rejected)
	}
}

func TestAnalytics_WithoutAnAPIKey_skipsTheCaptureAndAcks(t *testing.T) {
	t.Parallel()
	e := newEnv(t, testdata.Register, withoutAPIKey)
	a := e.append(t, userActor(e.ids.NewV7()))
	m := e.message(a)
	e.deliver(t, m)
	skipped := logged(t, e, "analytics.capture_skipped")
	line := map[string]any{}
	if len(skipped) == 1 {
		line = skipped[0]
	}
	if m.outcome != bus.OutcomeAck || e.fake.Received() != 0 || line["reason"] != "no_api_key" ||
		line["uuid"] != a.id.String() || line["event"] != testdata.ProbeEvent {
		t.Fatalf("verdict %q with %d received and skipped lines %v, want ack, none received and one no_api_key line",
			m.outcome, e.fake.Received(), skipped)
	}
	if sent := logged(t, e, "analytics.capture_sent"); len(sent) != 0 {
		t.Fatalf("capture_sent lines = %v, want none without an API key", sent)
	}
}

func exporting(c analytics.Capture, ok bool, err error) func(*analytics.Registry) {
	return func(r *analytics.Registry) {
		analytics.Export(r, string(events.TypeSystemPinged),
			func(context.Context, events.SystemPinged) (analytics.Capture, bool, error) { return c, ok, err })
	}
}

func TestAnalytics_Handler_turnsWhatTheMapperReturnsIntoAVerdict(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		register func(*analytics.Registry)
		want     verdict
	}{
		"skip": {exporting(analytics.Capture{}, false, nil), verdict{bus.OutcomeAck, "", 0}},
		"retryable error": {
			exporting(analytics.Capture{}, true, errs.New(errs.CodeUpstreamUnavailable, "test.mapper")),
			verdict{bus.OutcomeNak, "", bus.NakSchedule()[0]},
		},
		"other error": {
			exporting(analytics.Capture{}, true, errs.New(errs.CodeNotFound, "test.mapper")),
			verdict{bus.OutcomeTerm, string(errs.CodeNotFound), 0},
		},
		"leaking a banned key": {
			exporting(analytics.Capture{Event: "leaky", Properties: map[string]any{"email": "x"}}, true, nil),
			verdict{bus.OutcomeTerm, string(errs.CodeAnalyticsPII), 0},
		},
		"leaking a banned value": {
			exporting(analytics.Capture{Event: "leaky", Properties: map[string]any{"note": "a@b.co"}}, true, nil),
			verdict{bus.OutcomeTerm, string(errs.CodeAnalyticsPII), 0},
		},
		"no event name": {
			exporting(analytics.Capture{}, true, nil), verdict{bus.OutcomeTerm, string(errs.CodeInternal), 0},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t, tt.register)
			m := e.message(e.append(t, userActor(e.ids.NewV7())))
			e.deliver(t, m)
			rows, letters := 0, 0
			if tt.want.outcome == bus.OutcomeAck {
				rows = 1
			}
			if tt.want.outcome == bus.OutcomeTerm {
				letters = 1
			}
			if got := m.verdict(); got != tt.want || e.deliveries(t) != rows || len(e.deadLetters(t)) != letters ||
				e.fake.Received() != 0 {
				t.Fatalf("verdict %+v with %d delivery rows, %d dead letters, %d received; want %+v, %d, %d, none",
					got, e.deliveries(t), len(e.deadLetters(t)), e.fake.Received(), tt.want, rows, letters)
			}
		})
	}
}

func TestAnalytics_Handler_logsWhyTheMapperSkippedAnEvent(t *testing.T) {
	t.Parallel()
	e := newEnv(t, exporting(analytics.Capture{}, false, nil))
	e.deliver(t, e.message(e.append(t, userActor(e.ids.NewV7()))))
	skipped := logged(t, e, "analytics.capture_skipped")
	if len(skipped) != 1 || skipped[0]["reason"] != "mapper" || skipped[0]["subject"] != "system.pinged" {
		t.Fatalf("capture_skipped lines = %v, want one with reason mapper for system.pinged", skipped)
	}
}

func TestAnalytics_Handler_defaultsTheDistinctIDFromTheActor(t *testing.T) {
	t.Parallel()
	noProfile := map[string]any{"$process_person_profile": false}
	tests := map[string]struct {
		actor    func(user string) string
		mapped   analytics.Capture
		distinct func(user string) string
		props    map[string]any
	}{
		"a user acts": {
			func(u string) string { return "user:" + u },
			analytics.Capture{Event: "e"},
			func(u string) string { return u },
			map[string]any{},
		},
		"the system acts without properties": {
			func(string) string { return "system:cron" },
			analytics.Capture{Event: "e"},
			func(string) string { return "system" }, noProfile,
		},
		"an admin acts on top of the mapper's properties": {
			func(string) string { return "admin:a1" },
			analytics.Capture{Event: "e", Properties: map[string]any{"k": "v"}},
			func(string) string { return "system" },
			map[string]any{"k": "v", "$process_person_profile": false},
		},
		"an agent acts": {
			func(string) string { return "agent:trader" },
			analytics.Capture{Event: "e"},
			func(string) string { return "system" }, noProfile,
		},
		"the mapper names the distinct id": {
			func(string) string { return "system:cron" },
			analytics.Capture{Event: "e", DistinctID: "cabal-alpha"},
			func(string) string { return "cabal-alpha" },
			map[string]any{},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t, exporting(tt.mapped, true, nil))
			user := e.ids.NewV7().String()
			a := e.append(t, tt.actor(user))
			e.deliver(t, e.message(a))
			captures := e.fake.Captures()
			if len(captures) != 1 {
				t.Fatalf("captures = %+v, want one", captures)
			}
			sameCapture(t, captures[0], fakes.PostHogCapture{
				APIKey: apiKey, UUID: a.id, Event: "e", DistinctID: tt.distinct(user), Timestamp: a.created,
				Properties: tt.props,
			})
		})
	}
}

func TestAnalytics_Handler_failsInternallyWithoutTheEventIDOrItsRow(t *testing.T) {
	t.Parallel()
	e := newEnv(t, testdata.Register)
	spec := e.consumer.Handlers[0]
	apply := func(ctx context.Context) error {
		return e.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
			return spec.Apply(ctx, tx, events.SystemPinged{V: 1}, e.clock.Now())
		})
	}
	noID := apply(t.Context())
	missing := apply(observability.WithEventID(t.Context(), ids.EventIDFrom(e.ids.NewV7())))
	if errs.CodeOf(noID) != errs.CodeInternal || errs.CodeOf(missing) != errs.CodeInternal ||
		!errors.Is(missing, pgx.ErrNoRows) || e.fake.Received() != 0 {
		t.Fatalf("without an id: %v; with an unknown id: %v; want internal both, no rows for the second", noID, missing)
	}
}
