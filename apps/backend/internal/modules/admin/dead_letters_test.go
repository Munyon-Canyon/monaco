package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type letterFixture struct {
	bus    testkit.Bus
	pool   *pgxpool.Pool
	clock  *testkit.Clock
	ids    *testkit.IDs
	uow    *db.UnitOfWork
	reader *sdkmetric.ManualReader
	logs   *testkit.Logs
	poller *app.DeadLettersPoller
}

func newLetterFixture(t *testing.T) *letterFixture {
	t.Helper()
	f := &letterFixture{
		bus:    testkit.NATS(t),
		pool:   testkit.DB(t),
		clock:  testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)),
		ids:    testkit.NewIDs(1),
		reader: sdkmetric.NewManualReader(),
		logs:   &testkit.Logs{},
	}
	f.uow = db.New(f.pool, f.ids, f.clock)
	f.poller = app.NewDeadLettersPoller(app.DeadLettersDeps{
		Source:   f.bus.Conn,
		Events:   adapters.BusEvents{Conn: f.bus.Conn},
		UoW:      f.uow,
		Reads:    f.pool,
		IDs:      f.ids,
		Clock:    f.clock,
		Meter:    sdkmetric.NewMeterProvider(sdkmetric.WithReader(f.reader)).Meter("admin-test"),
		Interval: time.Second,
	})
	return f
}

func (f *letterFixture) ctx(t *testing.T) context.Context {
	t.Helper()
	ctx := observability.WithActor(t.Context(), "system:poller.admin.deadletters")
	return observability.WithLogger(ctx, observability.NewLogger(config.Config{Env: config.EnvTest}, f.logs))
}

func (f *letterFixture) put(t *testing.T, letter bus.DeadLetter) {
	t.Helper()
	body, err := json.Marshal(letter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.bus.JS.Publish(t.Context(), f.bus.Conn.Subject("deadletter."+letter.Consumer), body); err != nil {
		t.Fatal(err)
	}
}

func (f *letterFixture) termLetter(event uuid.UUID, code string) bus.DeadLetter {
	return bus.DeadLetter{
		Consumer: "analytics",
		Handler:  "analytics.posthog.follow.created",
		Subject:  f.bus.Conn.Subject("events.follow.created"),
		MsgID:    event.String(),
		Delivery: 1,
		Code:     code,
		Error:    "posthog.Capture: " + code,
		Headers:  map[string][]string{"Nats-Msg-Id": {event.String()}},
		Data:     json.RawMessage(`{"follower_id":"x"}`),
	}
}

func (f *letterFixture) marker(event uuid.UUID) bus.DeadLetter {
	return bus.DeadLetter{Consumer: "analytics", MsgID: event.String(), Code: "ok"}
}

func (f *letterFixture) tick(t *testing.T) (scanned, changed int) {
	t.Helper()
	report, err := f.poller.Tick(f.ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	return report.Scanned, report.Changed
}

type deadLetterRow struct {
	StreamSeq    int64
	Consumer     string
	Handler      string
	Subject      string
	EventID      *uuid.UUID
	Code         string
	Error        string
	Letter       string
	Occurrences  int32
	Status       string
	FirstSeenAt  time.Time
	LastSeenAt   time.Time
	RedrivenAt   *time.Time
	ResolvedAt   *time.Time
	ResolvedBy   *uuid.UUID
	ResolveReson *string
}

func (f *letterFixture) rows(t *testing.T) []deadLetterRow {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT stream_seq, consumer, handler, subject, event_id, code, error,
		letter::text, occurrences, status, first_seen_at, last_seen_at, redriven_at, resolved_at, resolved_by,
		resolve_reason FROM dead_letters ORDER BY stream_seq`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowToStructByPos[deadLetterRow])
	if err != nil {
		t.Fatal(err)
	}
	for i := range got {
		got[i].FirstSeenAt, got[i].LastSeenAt = got[i].FirstSeenAt.UTC(), got[i].LastSeenAt.UTC()
	}
	return got
}

func (f *letterFixture) summaries(t *testing.T) []deadLetterRow {
	t.Helper()
	got := f.rows(t)
	for i := range got {
		got[i].Letter, got[i].Error = "", ""
		got[i].FirstSeenAt, got[i].LastSeenAt = time.Time{}, time.Time{}
	}
	return got
}

func (f *letterFixture) gauge(t *testing.T) map[string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := f.reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	out := map[string]int64{}
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			g, ok := m.Data.(metricdata.Gauge[int64])
			if !ok || m.Name != "monaco_dead_letters" {
				continue
			}
			for _, p := range g.DataPoints {
				consumer, _ := p.Attributes.Value("consumer")
				out[consumer.AsString()] = p.Value
			}
		}
	}
	return out
}

func TestDeadLetters_RecordsATermedMessageAsAnOpenRowWithItsHandlerSubjectAndEventID(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	event := f.ids.NewV7()
	letter := f.termLetter(event, "posthog_rejected")
	f.put(t, letter)

	scanned, changed := f.tick(t)
	if scanned != 1 || changed != 1 {
		t.Fatalf("tick = scanned %d changed %d, want 1 and 1", scanned, changed)
	}
	got := f.rows(t)
	if len(got) != 1 {
		t.Fatalf("rows = %+v, want one", got)
	}
	row := got[0]
	want := deadLetterRow{
		StreamSeq: 1, Consumer: "analytics", Handler: letter.Handler, Subject: letter.Subject, EventID: &event,
		Code: "posthog_rejected", Error: letter.Error, Occurrences: 1, Status: "open",
		FirstSeenAt: f.clock.Now(), LastSeenAt: f.clock.Now(),
	}
	row.Letter = ""
	if !reflect.DeepEqual(row, want) {
		t.Fatalf("row = %+v, want %+v", row, want)
	}
	f.assertStoredVerbatim(t, got[0].Letter, letter)
	if !strings.Contains(string(f.logs.Bytes()), `"msg":"admin.dead_letter.recorded"`) ||
		!strings.Contains(string(f.logs.Bytes()), `"status":"open"`) {
		t.Fatalf("logs = %s, want admin.dead_letter.recorded with status open", f.logs.Bytes())
	}
	if scanned, changed := f.tick(t); scanned != 0 || changed != 0 {
		t.Fatalf("second tick = %d, %d, want nothing left to pull", scanned, changed)
	}
}

func (f *letterFixture) assertStoredVerbatim(t *testing.T, stored string, letter bus.DeadLetter) {
	t.Helper()
	var got bus.DeadLetter
	var data bytes.Buffer
	if err := json.Unmarshal([]byte(stored), &got); err != nil || json.Compact(&data, got.Data) != nil ||
		got.Code != letter.Code || data.String() != string(letter.Data) || got.EventID() != letter.EventID() {
		t.Fatalf("letter = %s (%v), want the DEADLETTER body verbatim", stored, err)
	}
}

func advisoryFor(consumer string, seq int) bus.DeadLetter {
	return bus.DeadLetter{
		Consumer: consumer, MsgID: consumer + "/" + strconv.Itoa(seq),
		Advisory: json.RawMessage(`{"stream_seq":` + strconv.Itoa(seq) + `}`),
	}
}

func TestDeadLetters_AdvisoryLetter(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	event := f.ids.NewV7()
	if err := f.bus.Conn.Publish(t.Context(), events.TypeSystemPinged.Subject(), json.RawMessage(`{"v":1}`),
		ids.EventIDFrom(event)); err != nil {
		t.Fatal(err)
	}
	f.put(t, advisoryFor("notify", 1))
	f.put(t, advisoryFor("notify", 9))

	if scanned, changed := f.tick(t); scanned != 2 || changed != 2 {
		t.Fatalf("tick = %d, %d, want 2 and 2", scanned, changed)
	}
	subject := f.bus.Conn.Subject(events.TypeSystemPinged.Subject())
	want := []deadLetterRow{
		{
			StreamSeq: 1, Consumer: "notify", Subject: subject, EventID: &event, Code: "max_deliveries",
			Occurrences: 1, Status: "open",
		},
		{StreamSeq: 2, Consumer: "notify", Code: "max_deliveries", Occurrences: 1, Status: "open"},
	}
	if got := f.summaries(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %+v, want max_deliveries rows, the second with a null event_id and an empty subject", got)
	}
}

type failingEvents struct{ err error }

func (f failingEvents) EventAt(context.Context, uint64) (app.Event, error) { return app.Event{}, f.err }

func TestRecordDeadLetter_failsAndWritesNothingWhenTheEventLookupFailsOrTheLetterCannotBeStored(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	down := app.NewRecordDeadLetter(
		f.uow,
		f.ids,
		f.clock,
		failingEvents{errs.New(errs.CodeUpstreamUnavailable, "nats")},
	)
	advisory := bus.DeadLetter{Consumer: "notify", Seq: 1, Advisory: json.RawMessage(`{"stream_seq":1}`)}
	if changed, err := down.Record(f.ctx(t), advisory); changed || errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Record with EVENTS down = %v, %v, want upstream_unavailable", changed, err)
	}
	unstorable := bus.DeadLetter{Consumer: "notify", Seq: 2, Data: json.RawMessage("{")}
	if changed, err := down.Record(f.ctx(t), unstorable); changed || errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Record of invalid JSON = %v, %v, want internal", changed, err)
	}
	if got := f.rows(t); len(got) != 0 {
		t.Fatalf("rows = %+v, want none", got)
	}
}

func TestDeadLetters_RedeliveredMessageRecordedOnce(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	event := f.ids.NewV7()
	f.put(t, f.termLetter(event, "posthog_rejected"))
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE dead_letters RENAME TO dead_letters_gone`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.poller.Tick(f.ctx(t)); err == nil {
		t.Fatal("tick succeeded with the table gone, want the sink to nak and fail")
	}
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE dead_letters_gone RENAME TO dead_letters`); err != nil {
		t.Fatal(err)
	}

	if scanned, changed := f.tick(t); scanned != 1 || changed != 1 {
		t.Fatalf("tick after the nak = %d, %d, want the redelivered letter recorded once", scanned, changed)
	}
	if got := f.rows(t); len(got) != 1 || got[0].Occurrences != 1 {
		t.Fatalf("rows = %+v, want one row with occurrences 1", got)
	}
}

func (f *letterFixture) redrive(t *testing.T) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(),
		`UPDATE dead_letters SET status = 'redriven', redriven_at = now(), resolved_by = $1, resolve_reason = 'fixed'`,
		f.ids.NewV7()); err != nil {
		t.Fatal(err)
	}
}

func TestDeadLetters_aTermAfterARedriveReopensTheLiveRowAndAReplayOfThatLetterChangesNothing(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	event := f.ids.NewV7()
	f.put(t, f.termLetter(event, "posthog_rejected"))
	f.tick(t)
	f.redrive(t)
	f.clock.Advance(time.Minute)
	again := f.termLetter(event, "upstream_unavailable")
	f.put(t, again)

	if scanned, changed := f.tick(t); scanned != 1 || changed != 1 {
		t.Fatalf("tick = %d, %d, want 1 and 1", scanned, changed)
	}
	f.assertReopened(t)
	again.Seq = 2
	record := app.NewRecordDeadLetter(f.uow, f.ids, f.clock, adapters.BusEvents{Conn: f.bus.Conn})
	if changed, err := record.Record(f.ctx(t), again); err != nil || changed {
		t.Fatalf("replay = %v, %v, want a letter seen once to change nothing", changed, err)
	}
	f.assertReopened(t)
}

func (f *letterFixture) assertReopened(t *testing.T) {
	t.Helper()
	got := f.rows(t)
	if len(got) != 1 || got[0].Occurrences != 2 || got[0].Status != "open" || got[0].Code != "upstream_unavailable" ||
		got[0].RedrivenAt == nil || got[0].StreamSeq != 2 || !got[0].LastSeenAt.After(got[0].FirstSeenAt) {
		t.Fatalf("rows = %+v, want the same row open again with occurrences 2 and redriven_at kept", got)
	}
}

func TestDeadLetters_aTermForAnEventWithNoIDOrAResolvedRowStartsANewRow(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	event := f.ids.NewV7()
	anonymous := f.termLetter(event, "invalid_input")
	anonymous.Headers = nil
	f.put(t, anonymous)
	f.put(t, f.termLetter(event, "invalid_input"))
	f.tick(t)
	if _, err := f.pool.Exec(
		t.Context(),
		`UPDATE dead_letters SET status = 'resolved' WHERE event_id IS NOT NULL`,
	); err != nil {
		t.Fatal(err)
	}
	f.put(t, f.termLetter(event, "invalid_input"))
	f.tick(t)

	got := f.rows(t)
	if len(got) != 3 || got[0].EventID != nil || got[1].Status != "resolved" || got[2].Status != "open" ||
		got[2].Occurrences != 1 {
		t.Fatalf("rows = %+v, want an id-less row, the resolved row and a fresh open row", got)
	}
}

func TestDeadLetters_aMarkerResolvesTheLiveRowOnceAndOtherMarkersChangeNothing(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	event := f.ids.NewV7()
	f.put(t, f.termLetter(event, "posthog_rejected"))
	f.tick(t)
	f.clock.Advance(time.Minute)
	f.put(t, f.marker(event))
	f.put(t, f.marker(event))
	f.put(t, f.marker(f.ids.NewV7()))
	f.put(t, bus.DeadLetter{Consumer: "analytics", MsgID: "not-a-uuid", Code: "ok"})

	if scanned, changed := f.tick(t); scanned != 4 || changed != 1 {
		t.Fatalf("tick = %d, %d, want 4 markers scanned and one row changed", scanned, changed)
	}
	got := f.rows(t)
	if len(got) != 1 || got[0].Status != "resolved" || got[0].ResolvedAt == nil ||
		!got[0].ResolvedAt.Equal(f.clock.Now()) || got[0].Occurrences != 1 {
		t.Fatalf("rows = %+v, want the row resolved at the marker's tick", got)
	}
	if !strings.Contains(string(f.logs.Bytes()), `"status":"resolved"`) {
		t.Fatalf("logs = %s, want admin.dead_letter.recorded with status resolved", f.logs.Bytes())
	}
}

func TestDeadLetters_theGaugeCountsOpenAndRedrivenRowsPerConsumer(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	for i, status := range []string{"open", "open", "redriven", "resolved", "discarded"} {
		consumer := "analytics"
		if i == 2 {
			consumer = "notify"
		}
		if _, err := f.pool.Exec(t.Context(), `INSERT INTO dead_letters (id, stream_seq, consumer, code, letter,
			status, first_seen_at, last_seen_at) VALUES ($1, $2, $3, 'x', '{}', $4, now(), now())`,
			f.ids.NewV7(), i+1, consumer, status); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.gauge(t); len(got) != 2 || got["analytics"] != 2 || got["notify"] != 1 {
		t.Fatalf("monaco_dead_letters = %v, want analytics 2 and notify 1", got)
	}
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE dead_letters RENAME TO dead_letters_gone`); err != nil {
		t.Fatal(err)
	}
	var rm metricdata.ResourceMetrics
	if err := f.reader.Collect(t.Context(), &rm); err == nil {
		t.Fatal("collect succeeded with the table gone, want the callback's error")
	}
}

func TestDeadLettersPoller_namesItselfAndFailsATickWhenTheStreamIsGone(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	if f.poller.Name() != "admin.deadletters" || f.poller.Interval() != time.Second {
		t.Fatalf("poller = %s every %s, want admin.deadletters every 1s", f.poller.Name(), f.poller.Interval())
	}
	if err := f.bus.JS.DeleteStream(t.Context(), f.bus.DeadLetter); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = f.bus.Conn.Apply(context.WithoutCancel(t.Context())) })
	if _, err := f.poller.Tick(f.ctx(t)); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("tick without DEADLETTER = %v, want upstream_unavailable", err)
	}
}

func TestModule_pollsDeadLettersOnTheConfiguredInterval(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	m := admin.New(module.Deps{
		Config: config.Config{Admin: config.Admin{DeadLettersInterval: 7 * time.Second}},
		Clock:  f.clock, IDs: f.ids, Pool: f.pool, UoW: f.uow, Bus: f.bus.Conn,
	})
	pollers := m.Pollers()
	if m.Name() != "admin" || len(pollers) != 1 || pollers[0].Name() != "admin.deadletters" ||
		pollers[0].Interval() != 7*time.Second {
		t.Fatalf("pollers = %v, want admin.deadletters every 7s", pollers)
	}
}
