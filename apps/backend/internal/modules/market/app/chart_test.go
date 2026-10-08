package app

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type chartRead struct {
	asset    func() (sqlc.Asset, error)
	newest   func() ([]sqlc.NewestSamplesRow, error)
	earliest func() (time.Time, error)
	buckets  func() ([]sqlc.ChartBucketsRow, error)
	queue    func() (int64, error)
	got      *sqlc.ChartBucketsParams
}

func (r chartRead) InsertPendingBackfills(context.Context, sqlc.InsertPendingBackfillsParams) (int64, error) {
	if r.queue == nil {
		return 0, nil
	}
	return r.queue()
}

func (r chartRead) AssetBySymbol(context.Context, string) (sqlc.Asset, error) { return r.asset() }

func (r chartRead) NewestSamples(context.Context, sqlc.NewestSamplesParams) ([]sqlc.NewestSamplesRow, error) {
	return r.newest()
}

func (r chartRead) EarliestPrice(context.Context, string) (time.Time, error) { return r.earliest() }

func (r chartRead) ChartBuckets(_ context.Context, arg sqlc.ChartBucketsParams) ([]sqlc.ChartBucketsRow, error) {
	if r.got != nil {
		*r.got = arg
	}
	return r.buckets()
}

type chartCase struct {
	read  chartRead
	raw   string
	code  errs.Code
	empty bool
}

func TestChart_stopsAtTheAcceptedSample(t *testing.T) {
	t.Parallel()
	apple := appleRow(t, "equity")
	when := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	acceptedAt := when.Add(-time.Minute)
	var got sqlc.ChartBucketsParams
	read := chartRead{
		asset: func() (sqlc.Asset, error) { return apple, nil },
		newest: func() ([]sqlc.NewestSamplesRow, error) {
			return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: acceptedAt, PriceMicros: 100}}, nil
		},
		buckets: func() ([]sqlc.ChartBucketsRow, error) {
			return []sqlc.ChartBucketsRow{{
				Bucket: acceptedAt, OpenMicros: 1, HighMicros: 1, LowMicros: 1, CloseMicros: 1,
			}}, nil
		},
		got: &got,
	}
	_, err := (&AssetChart{read: read, clock: testkit.NewClock(when)}).Handle(t.Context(), "AAPLx", "1D")
	if err != nil || !got.Until.Equal(acceptedAt) || got.Since.Equal(when) {
		t.Fatalf("until = %s since %s err %v", got.Until, got.Since, err)
	}
}

func TestChart_reportsAMissingAssetAndABadRead(t *testing.T) {
	t.Parallel()
	apple := appleRow(t, "equity")
	when := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	boom := errs.New(errs.CodeInternal, "market.test")
	okAsset := func() (sqlc.Asset, error) { return apple, nil }
	sample := func() ([]sqlc.NewestSamplesRow, error) {
		return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: when.Add(-time.Minute), PriceMicros: 110}}, nil
	}
	bar := sqlc.ChartBucketsRow{Bucket: when, OpenMicros: 1, HighMicros: 4, LowMicros: 1, CloseMicros: 3}
	cases := []chartCase{
		{raw: "nope", code: errs.CodeInvalidInput},
		{
			raw: "1D",
			read: chartRead{asset: func() (sqlc.Asset, error) {
				return sqlc.Asset{}, sql.ErrNoRows
			}},
			code: errs.CodeAssetNotFound,
		},
		{raw: "1D", read: chartRead{asset: func() (sqlc.Asset, error) {
			return sqlc.Asset{}, boom
		}}, code: errs.CodeInternal},
		{raw: "1D", read: chartRead{asset: okAsset, newest: func() ([]sqlc.NewestSamplesRow, error) {
			return nil, boom
		}}, code: errs.CodeInternal},
		{raw: "1D", read: chartRead{asset: okAsset, newest: func() ([]sqlc.NewestSamplesRow, error) {
			return []sqlc.NewestSamplesRow{{Ts: when, PriceMicros: -1}}, nil
		}}, code: errs.CodeDecodeFailed},
		{raw: "1D", read: chartRead{asset: okAsset, newest: func() ([]sqlc.NewestSamplesRow, error) {
			return nil, nil
		}}, empty: true},
		{raw: "ALL", read: chartRead{asset: okAsset, newest: sample, earliest: func() (time.Time, error) {
			return time.Time{}, sql.ErrNoRows
		}}, empty: true},
		{raw: "ALL", read: chartRead{asset: okAsset, newest: sample, earliest: func() (time.Time, error) {
			return time.Time{}, boom
		}}, code: errs.CodeInternal},
		{raw: "1D", read: chartRead{asset: okAsset, newest: sample, buckets: func() ([]sqlc.ChartBucketsRow, error) {
			return nil, boom
		}}, code: errs.CodeInternal},
		{raw: "1D", read: chartRead{asset: okAsset, newest: sample, buckets: func() ([]sqlc.ChartBucketsRow, error) {
			bad := sqlc.ChartBucketsRow{Bucket: when, OpenMicros: 1, HighMicros: 1, LowMicros: 1, CloseMicros: -1}
			return []sqlc.ChartBucketsRow{bad}, nil
		}}, code: errs.CodeDecodeFailed},
		{raw: "1D", read: chartRead{asset: okAsset, newest: func() ([]sqlc.NewestSamplesRow, error) {
			return []sqlc.NewestSamplesRow{{Ts: when.Add(-400 * 24 * time.Hour), PriceMicros: 50}}, nil
		}, buckets: func() ([]sqlc.ChartBucketsRow, error) { return nil, nil }}, empty: true},
		{raw: "1D", read: chartRead{asset: okAsset, newest: func() ([]sqlc.NewestSamplesRow, error) {
			return []sqlc.NewestSamplesRow{
				{Ts: when.Add(-time.Hour), PriceMicros: 50},
				{Ts: when, PriceMicros: 400},
			}, nil
		}, buckets: func() ([]sqlc.ChartBucketsRow, error) { return []sqlc.ChartBucketsRow{bar}, nil }}},
	}
	for i, tc := range cases {
		got, err := (&AssetChart{read: tc.read, clock: testkit.NewClock(when)}).Handle(t.Context(), "AAPLx", tc.raw)
		assertChartCase(t, i, tc, got, err)
	}
}

func assertChartCase(t *testing.T, i int, tc chartCase, got Chart, err error) {
	t.Helper()
	if tc.code != "" {
		if errs.CodeOf(err) != tc.code {
			t.Fatalf("case %d = %v, want %s", i, err, tc.code)
		}
		return
	}
	if err != nil || got.Empty != tc.empty {
		t.Fatalf("case %d = %+v %v", i, got, err)
	}
	if tc.empty {
		return
	}
	if len(got.Points) != 1 || got.Bucket != 5*time.Minute {
		t.Fatalf("case %d points = %+v", i, got.Points)
	}
	point := got.Points[0]
	if point.Open.Uint64() != 1 || point.High.Uint64() != 4 {
		t.Fatalf("case %d ohlc = %+v", i, point)
	}
}

func TestChart_aFailedBackfillQueueIsLoggedAndTheChartStillReturns(t *testing.T) {
	t.Parallel()
	apple := appleRow(t, "equity")
	when := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	bar := sqlc.ChartBucketsRow{Bucket: when, OpenMicros: 1, HighMicros: 4, LowMicros: 1, CloseMicros: 3}
	read := chartRead{
		asset: func() (sqlc.Asset, error) { return apple, nil },
		newest: func() ([]sqlc.NewestSamplesRow, error) {
			return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: when, PriceMicros: 3}}, nil
		},
		buckets: func() ([]sqlc.ChartBucketsRow, error) { return []sqlc.ChartBucketsRow{bar}, nil },
		queue:   func() (int64, error) { return 0, errs.New(errs.CodeInternal, "market.test") },
	}
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	got, err := (&AssetChart{read: read, clock: testkit.NewClock(when)}).Handle(ctx, "AAPLx", "1D")
	if err != nil || got.Empty || len(got.Points) != 1 || got.Points[0].Close != money.MicrosFromUint64(3) {
		t.Fatalf("chart = %+v, %v, want the one bar despite the failed queue write", got, err)
	}
	if !strings.Contains(string(logs.Bytes()), `"msg":"market.chart.backfill_queue_failed"`) {
		t.Fatalf("logs = %s, want market.chart.backfill_queue_failed", logs.Bytes())
	}
}
