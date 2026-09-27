package bus_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const pinged = "events.system.pinged"

func TestPublish_sameMsgIDTwiceStoresOneMessage(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	gen := testkit.NewIDs(1)
	first, second := eventID(t, gen), eventID(t, gen)

	for _, id := range []ids.EventID{first, first, second} {
		if err := b.Conn.Publish(t.Context(), pinged, []byte(`{"v":1}`), id); err != nil {
			t.Fatal(err)
		}
	}
	if n := msgs(t, b); n != 2 {
		t.Fatalf("EVENTS holds %d messages after publishing ids a, a, b; want 2", n)
	}
}

func TestPublish_setsMsgIDAndTraceHeaders(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	id := eventID(t, testkit.NewIDs(2))
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatal(err)
	}
	ctx := trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(
		trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled}))

	if err := b.Conn.Publish(ctx, pinged, []byte(`{"v":1}`), id); err != nil {
		t.Fatal(err)
	}
	s, err := b.JS.Stream(t.Context(), b.Events)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := s.GetMsg(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Subject != b.Conn.Subject(pinged) || !bytes.Equal(msg.Data, []byte(`{"v":1}`)) ||
		msg.Header.Get(jetstream.MsgIDHeader) != id.String() ||
		len(
			msg.Header["traceparent"],
		) != 1 || msg.Header["traceparent"][0] != "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" {
		t.Fatalf("stored %s %q headers %v", msg.Subject, msg.Data, msg.Header)
	}
}

func TestPublish_fullStreamIsRetryable(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	s, err := b.JS.Stream(t.Context(), b.Events)
	if err != nil {
		t.Fatal(err)
	}
	tiny := s.CachedInfo().Config
	tiny.MaxBytes = 1024
	if _, err := b.JS.UpdateStream(t.Context(), tiny); err != nil {
		t.Fatal(err)
	}
	gen := testkit.NewIDs(3)
	payload := bytes.Repeat([]byte("x"), 600)
	if err := b.Conn.Publish(t.Context(), pinged, payload, eventID(t, gen)); err != nil {
		t.Fatal(err)
	}

	err = b.Conn.Publish(t.Context(), pinged, payload, eventID(t, gen))
	if code := errs.CodeOf(err); code != errs.CodeUpstreamUnavailable || !errs.Retryable(code) {
		t.Fatalf("publish into a full stream = %v, want retryable upstream_unavailable", err)
	}
	if n := msgs(t, b); n != 1 {
		t.Fatalf("full stream holds %d messages, want the 1 that fit", n)
	}
}

func TestPublish_subjectNoStreamTakesIsRetryable(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	err := b.Conn.Publish(t.Context(), "events.nobody.listens", nil, eventID(t, testkit.NewIDs(4)))
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("publish with no stream = %v, want upstream_unavailable", err)
	}
}

func TestPublish_closedConnectionAndExpiredDeadlineAreRetryable(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	gen := testkit.NewIDs(7)
	expired, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
	defer cancel()
	if err := b.Conn.Publish(expired, pinged, nil, eventID(t, gen)); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("publish past its deadline = %v, want upstream_unavailable", err)
	}
	b.Conn.Close(t.Context())
	if err := b.Conn.Publish(
		t.Context(),
		pinged,
		nil,
		eventID(t, gen),
	); errs.CodeOf(
		err,
	) != errs.CodeUpstreamUnavailable {
		t.Fatalf("publish on a closed connection = %v, want upstream_unavailable", err)
	}
}

func TestPublish_otherJetStreamErrorsAreInternal(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	s, err := b.JS.Stream(t.Context(), b.Events)
	if err != nil {
		t.Fatal(err)
	}
	sealed := s.CachedInfo().Config
	sealed.Sealed = true
	if _, err := b.JS.UpdateStream(t.Context(), sealed); err != nil {
		t.Fatal(err)
	}
	err = b.Conn.Publish(t.Context(), pinged, nil, eventID(t, testkit.NewIDs(5)))
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("publish into a sealed stream = %v, want internal", err)
	}
}

func TestPublishHint_deliversOnCoreNATSAndCountsDrops(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	b := testkit.NATS(t, testkit.WithBusOptions(
		bus.WithMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))))
	sub, err := nats.Connect(testkit.NATSURL())
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	got := make(chan *nats.Msg, 1)
	if _, err := sub.ChanSubscribe(b.Conn.Subject("hint.>"), got); err != nil {
		t.Fatal(err)
	}
	if err := sub.Flush(); err != nil {
		t.Fatal(err)
	}

	b.Conn.PublishHint(t.Context(), "cabal.42.updated", []byte("42"))
	select {
	case msg := <-got:
		if msg.Subject != b.Conn.Subject("hint.cabal.42.updated") || string(msg.Data) != "42" {
			t.Fatalf("hint = %s %q", msg.Subject, msg.Data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hint not delivered within 5s")
	}
	if n := counter(t, reader, "monaco_bus_hint_dropped_total"); n != 0 {
		t.Fatalf("dropped = %d after a delivered hint, want 0", n)
	}
	b.Conn.Close(t.Context())
	b.Conn.PublishHint(t.Context(), "cabal.42.updated", []byte("42"))
	if n := counter(t, reader, "monaco_bus_hint_dropped_total"); n != 1 {
		t.Fatalf("dropped = %d after a hint on a closed connection, want 1", n)
	}
}

func TestExportAccountGauges_reportsStreamsAndStorage(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	b := testkit.NATS(t, testkit.WithBusOptions(
		bus.WithMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))))
	if err := b.Conn.Publish(t.Context(), pinged, []byte(`{"v":1}`), eventID(t, testkit.NewIDs(6))); err != nil {
		t.Fatal(err)
	}
	unregister, err := b.Conn.ExportAccountGauges()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unregister() }()

	if n := gauge(t, reader, "monaco_bus_account_streams"); n < 2 {
		t.Fatalf("streams gauge = %d, want at least this test's 2", n)
	}
	if n := gauge(t, reader, "monaco_bus_account_storage_bytes"); n <= 0 {
		t.Fatalf("storage gauge = %d after a publish, want > 0", n)
	}
	if n := gauge(t, reader, "monaco_bus_account_storage_limit_bytes"); n != -1 {
		t.Fatalf("storage limit gauge = %d, want -1 (unlimited) on the embedded server", n)
	}
	if n := gauge(t, reader, "monaco_bus_account_consumers"); n < 0 {
		t.Fatalf("consumers gauge = %d, want >= 0", n)
	}
}

func TestExportAccountGauges_collectionFailsWhenTheConnectionIsClosed(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	conn, err := bus.Connect(t.Context(), config.NATS{URL: testkit.NATSURL()}, bus.ProcessWorker,
		bus.WithMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExportAccountGauges(); err != nil {
		t.Fatal(err)
	}
	conn.Close(t.Context())
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("collect on a closed connection = %v, want upstream_unavailable", err)
	}
}

func eventID(t *testing.T, gen *testkit.IDs) ids.EventID {
	t.Helper()
	id, err := ids.ParseEventID(gen.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func msgs(t *testing.T, b testkit.Bus) uint64 {
	t.Helper()
	s, err := b.JS.Stream(t.Context(), b.Events)
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.Info(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return info.State.Msgs
}

func collect(t *testing.T, reader *sdkmetric.ManualReader, name string) metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m
			}
		}
	}
	return metricdata.Metrics{}
}

func counter(t *testing.T, reader *sdkmetric.ManualReader, name string) int64 {
	t.Helper()
	sum, ok := collect(t, reader, name).Data.(metricdata.Sum[int64])
	if !ok {
		return 0
	}
	var total int64
	for _, p := range sum.DataPoints {
		total += p.Value
	}
	return total
}

func gauge(t *testing.T, reader *sdkmetric.ManualReader, name string) int64 {
	t.Helper()
	g, ok := collect(t, reader, name).Data.(metricdata.Gauge[int64])
	if !ok || len(g.DataPoints) != 1 {
		t.Fatalf("gauge %s missing", name)
	}
	return g.DataPoints[0].Value
}

func TestPublish_accountStorageCapIsRetryable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srv, err := natsserver.NewServer(&natsserver.Options{
		Host: "127.0.0.1", Port: natsserver.RANDOM_PORT, NoLog: true, NoSigs: true,
		JetStream: true, StoreDir: dir, JetStreamMaxStore: 2 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.Start()
	t.Cleanup(func() {
		srv.Shutdown()
		srv.WaitForShutdown()
	})
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("capped nats-server not ready")
	}
	conn, err := bus.Connect(t.Context(), config.NATS{URL: srv.ClientURL()}, bus.ProcessWorker)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	admin, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	js, err := jetstream.New(admin)
	if err != nil {
		t.Fatal(err)
	}
	uncapped := bus.Streams()[0]
	uncapped.MaxBytes = -1
	if _, err := js.CreateStream(t.Context(), uncapped); err != nil {
		t.Fatal(err)
	}

	gen := testkit.NewIDs(8)
	payload := bytes.Repeat([]byte("x"), 256<<10)
	for range 16 {
		err = conn.Publish(t.Context(), pinged, payload, eventID(t, gen))
		if err != nil {
			break
		}
	}
	if code := errs.CodeOf(err); code != errs.CodeUpstreamUnavailable {
		t.Fatalf("publish past the account storage cap = %v, want retryable upstream_unavailable", err)
	}
}
