package social_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestReportAlert_logsTheReportWithALinkToTheAdminPanel(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	ctx := observability.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&logs, nil)))
	gen := testkit.NewIDs(7)
	report, target := gen.NewV7(), gen.NewV7()
	e := events.ReportCreated{V: 1, ReportID: report, Kind: "message", TargetID: target, Reason: "abuse"}
	if err := (adapters.ReportAlert{}).Handle(ctx, db.Tx{}, e, time.Time{}); err != nil {
		t.Fatalf("Handle = %v, want nil", err)
	}
	logged := logs.String()
	for _, want := range []string{
		"social.report_received", report.String(), target.String(), `"kind":"message"`, `"reason":"abuse"`,
		adapters.ReportsLink + "?report=" + report.String(),
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("log %s lacks %q", logged, want)
		}
	}
}

func gaugeValues(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if g, ok := m.Data.(metricdata.Gauge[int64]); ok && len(g.DataPoints) == 1 {
				got[m.Name] = g.DataPoints[0].Value
			}
		}
	}
	return got
}

func TestReportGauges_countOpenReportsAndTheAgeOfTheOldest(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("report_gauges_test")
	adapters.ExportReportGauges(meter, f.pool, f.clock)
	want := map[string]int64{"monaco_reports_open": 0, "monaco_reports_oldest_open_seconds": 0}
	if got := gaugeValues(
		t,
		reader,
	); got["monaco_reports_open"] != 0 ||
		got["monaco_reports_oldest_open_seconds"] != 0 {
		t.Fatalf("gauges with no reports = %v, want %v", got, want)
	}
	first, err := f.file(t, f.reporter, domain.ReportMessage, f.message)
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(90 * time.Minute)
	if _, err := f.file(t, f.reporter, domain.ReportUser, f.target.UUID()); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(30 * time.Minute)
	got := gaugeValues(t, reader)
	if got["monaco_reports_open"] != 2 || got["monaco_reports_oldest_open_seconds"] != int64(2*time.Hour/time.Second) {
		t.Fatalf("gauges = %v, want 2 open and an oldest of 7200 s", got)
	}
	if err := f.resolveAs(asAdmin(t), first, "handled"); err != nil {
		t.Fatal(err)
	}
	got = gaugeValues(t, reader)
	if got["monaco_reports_open"] != 1 ||
		got["monaco_reports_oldest_open_seconds"] != int64(30*time.Minute/time.Second) {
		t.Fatalf("gauges after resolving the oldest = %v, want 1 open and an oldest of 1800 s", got)
	}
}

func TestReportGauges_failTheCollectionWhenTheCountFails(t *testing.T) {
	t.Parallel()
	f := newReportFixture(t)
	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("report_gauges_failing_test")
	adapters.ExportReportGauges(meter, f.pool, f.clock)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE reports`); err != nil {
		t.Fatal(err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Collect = %v, want %s", err, errs.CodeDBUnavailable)
	}
}
