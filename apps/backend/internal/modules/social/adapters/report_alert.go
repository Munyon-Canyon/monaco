package adapters

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const ReportsLink = "https://monacolabs.retool.com/reports"

type ReportAlert struct{}

func (ReportAlert) Handle(ctx context.Context, _ db.Tx, e events.ReportCreated, _ time.Time) error {
	observability.Info(ctx, observability.SocialReportReceived,
		slog.String("report_id", e.ReportID.String()), slog.String("kind", e.Kind),
		slog.String("target_id", e.TargetID.String()), slog.String("reason", e.Reason),
		slog.String("link", ReportsLink+"?report="+e.ReportID.String()))
	return nil
}

type OpenReports interface {
	CountOpenReports(ctx context.Context, now time.Time) (sqlc.CountOpenReportsRow, error)
}

type reportCounts struct {
	reads sqlc.DBTX
}

func (r reportCounts) CountOpenReports(ctx context.Context, now time.Time) (sqlc.CountOpenReportsRow, error) {
	return sqlc.New(r.reads).CountOpenReports(ctx, now)
}

func ExportReportGauges(meter metric.Meter, reads sqlc.DBTX, clk clock.Clock) error {
	return exportReportGauges(meter, reportCounts{reads: reads}, clk)
}

func exportReportGauges(meter metric.Meter, source OpenReports, clk clock.Clock) error {
	const op = "social.ExportReportGauges"
	open, err := meter.Int64ObservableGauge("monaco_reports_open",
		metric.WithUnit("{report}"), metric.WithDescription("User reports not yet resolved."))
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	oldest, err := meter.Int64ObservableGauge("monaco_reports_oldest_open_seconds",
		metric.WithUnit("s"), metric.WithDescription("Age of the oldest unresolved report; 0 if none."))
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	_, err = meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		row, err := source.CountOpenReports(ctx, clk.Now().UTC())
		if err != nil {
			return errs.Wrap(err, errs.CodeDBUnavailable, op)
		}
		o.ObserveInt64(open, row.OpenCount)
		o.ObserveInt64(oldest, row.OldestSeconds)
		return nil
	}, open, oldest)
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	return nil
}
