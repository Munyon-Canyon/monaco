package app

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
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
	newestAt func(time.Time) []sqlc.NewestSamplesRow
	first    []sqlc.FirstSamplesSinceRow
}

func (r chartRead) FirstSamplesSince(
	context.Context, sqlc.FirstSamplesSinceParams,
) ([]sqlc.FirstSamplesSinceRow, error) {
	return r.first, nil
}

func (r chartRead) InsertPendingBackfills(context.Context, sqlc.InsertPendingBackfillsParams) (int64, error) {
	if r.queue == nil {
		return 0, nil
	}
	return r.queue()
}

func (r chartRead) AssetBySymbol(context.Context, string) (sqlc.Asset, error) { return r.asset() }

func (r chartRead) NewestSamples(_ context.Context, arg sqlc.NewestSamplesParams) ([]sqlc.NewestSamplesRow, error) {
	if r.newestAt != nil {
		return r.newestAt(arg.At), nil
	}
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
	_, err := (&AssetChart{read: read, base: read, clock: testkit.NewClock(when)}).Handle(t.Context(), "AAPLx", "1D")
	if err != nil || !got.Until.Equal(acceptedAt) || got.Since.Equal(when) {
		t.Fatalf("until = %s since %s err %v", got.Until, got.Since, err)
	}
}

func TestChart_dayChartIsTheLastSessionWhenTheMarketIsClosed(t *testing.T) {
	t.Parallel()
	apple := appleRow(t, "equity")
	saturday := time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)
	fridayOpen := time.Date(2026, 3, 6, 14, 30, 0, 0, time.UTC)
	var got sqlc.ChartBucketsParams
	read := chartRead{
		asset: func() (sqlc.Asset, error) { return apple, nil },
		newest: func() ([]sqlc.NewestSamplesRow, error) {
			return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: saturday.Add(-time.Minute), PriceMicros: 100}}, nil
		},
		buckets: func() ([]sqlc.ChartBucketsRow, error) { return nil, nil },
		got:     &got,
	}
	chart := &AssetChart{read: read, base: read, clock: testkit.NewClock(saturday)}
	_, err := chart.Handle(t.Context(), "AAPLx", "1D")
	if err != nil || !got.Since.Equal(fridayOpen) {
		t.Fatalf("since = %s want %s err %v", got.Since, fridayOpen, err)
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
		chart := &AssetChart{read: tc.read, base: tc.read, clock: testkit.NewClock(when)}
		got, err := chart.Handle(t.Context(), "AAPLx", tc.raw)
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
	got, err := (&AssetChart{read: read, base: read, clock: testkit.NewClock(when)}).Handle(ctx, "AAPLx", "1D")
	if err != nil || got.Empty || len(got.Points) != 1 || got.Points[0].Close != money.MicrosFromUint64(3) {
		t.Fatalf("chart = %+v, %v, want the one bar despite the failed queue write", got, err)
	}
	if !strings.Contains(string(logs.Bytes()), `"msg":"market.chart.backfill_queue_failed"`) {
		t.Fatalf("logs = %s, want market.chart.backfill_queue_failed", logs.Bytes())
	}
}

func TestChart_dayChartCarriesThePreviousCloseAndOtherRangesOmitIt(t *testing.T) {
	t.Parallel()
	apple := appleRow(t, "equity")
	when := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	info, err := domain.Session(domain.KindEquity, when)
	if err != nil {
		t.Fatal(err)
	}
	closedAt := info.LastClose.Add(-time.Minute)
	read := chartRead{
		asset: func() (sqlc.Asset, error) { return apple, nil },
		newestAt: func(at time.Time) []sqlc.NewestSamplesRow {
			if at.Equal(info.LastClose) {
				return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: closedAt, PriceMicros: 98_000_000}}
			}
			return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: when.Add(-time.Minute), PriceMicros: 100_000_000}}
		},
		buckets: func() ([]sqlc.ChartBucketsRow, error) {
			return []sqlc.ChartBucketsRow{{
				Bucket: when.Add(-time.Minute), OpenMicros: 1, HighMicros: 1, LowMicros: 1, CloseMicros: 1,
			}}, nil
		},
	}
	chart := &AssetChart{read: read, base: read, clock: testkit.NewClock(when)}
	day, err := chart.Handle(t.Context(), "AAPLx", "1D")
	if err != nil || day.PreviousClose == nil || *day.PreviousClose != money.MicrosFromUint64(98_000_000) {
		t.Fatalf("1D previous close = %v err %v", day.PreviousClose, err)
	}
	for _, raw := range []string{"1W", "1M", "3M", "1Y"} {
		other, otherErr := chart.Handle(t.Context(), "AAPLx", raw)
		if otherErr != nil || other.PreviousClose != nil {
			t.Fatalf("%s previous close = %v err %v", raw, other.PreviousClose, otherErr)
		}
	}
}

func TestChart_dayChartOmitsThePreviousCloseWhenTheSampleIsMissing(t *testing.T) {
	t.Parallel()
	apple := appleRow(t, "equity")
	when := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	read := chartRead{
		asset: func() (sqlc.Asset, error) { return apple, nil },
		newestAt: func(at time.Time) []sqlc.NewestSamplesRow {
			if at.Equal(when) {
				return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: when.Add(-time.Minute), PriceMicros: 100_000_000}}
			}
			return nil
		},
		buckets: func() ([]sqlc.ChartBucketsRow, error) {
			return []sqlc.ChartBucketsRow{{
				Bucket: when.Add(-time.Minute), OpenMicros: 1, HighMicros: 1, LowMicros: 1, CloseMicros: 1,
			}}, nil
		},
	}
	day, err := (&AssetChart{read: read, base: read, clock: testkit.NewClock(when)}).Handle(t.Context(), "AAPLx", "1D")
	if err != nil || day.PreviousClose != nil || day.Empty {
		t.Fatalf("previous close = %v empty %v err %v", day.PreviousClose, day.Empty, err)
	}
}

func TestChart_dayChartReportsEachPreviousCloseFailure(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "market.test")
	when := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	farFuture := time.Date(2099, 3, 4, 15, 0, 0, 0, time.UTC)
	apple := appleRow(t, "equity")
	pre := appleRow(t, "pre_ipo")
	cases := map[string]struct {
		row   sqlc.Asset
		at    time.Time
		close func(time.Time) []sqlc.NewestSamplesRow
		first []sqlc.FirstSamplesSinceRow
		fail  bool
	}{
		"no calendar": {row: apple, at: farFuture},
		"bad close sample": {row: apple, at: when, close: func(at time.Time) []sqlc.NewestSamplesRow {
			if at.Equal(when) {
				return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: when, PriceMicros: 1}}
			}
			return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: when, PriceMicros: -1}}
		}},
		"close read fails": {row: apple, at: when, fail: true},
		"bad open sample": {
			row: pre, at: when,
			first: []sqlc.FirstSamplesSinceRow{{Mint: pre.Mint, Ts: when, PriceMicros: -1}},
		},
		"open read fails": {row: pre, at: when, fail: true},
	}
	for name, tc := range cases {
		read := dayBaseFake{chartRead: chartRead{
			asset: func() (sqlc.Asset, error) { return tc.row, nil },
			newest: func() ([]sqlc.NewestSamplesRow, error) {
				return []sqlc.NewestSamplesRow{{Mint: tc.row.Mint, Ts: tc.at, PriceMicros: 1}}, nil
			},
			buckets: func() ([]sqlc.ChartBucketsRow, error) {
				return []sqlc.ChartBucketsRow{
					{Bucket: tc.at, OpenMicros: 1, HighMicros: 1, LowMicros: 1, CloseMicros: 1},
				}, nil
			},
		}, close: tc.close, first: tc.first, boom: boom, now: tc.at, fail: tc.fail}
		chart := &AssetChart{read: read, base: read, clock: testkit.NewClock(tc.at)}
		if _, err := chart.Handle(t.Context(), "AAPLx", "1D"); err == nil {
			t.Fatalf("%s: want an error", name)
		}
	}
}

type dayBaseFake struct {
	chartRead
	close func(time.Time) []sqlc.NewestSamplesRow
	first []sqlc.FirstSamplesSinceRow
	boom  error
	now   time.Time
	fail  bool
}

func (r dayBaseFake) NewestSamples(_ context.Context, arg sqlc.NewestSamplesParams) ([]sqlc.NewestSamplesRow, error) {
	if r.close == nil {
		if r.fail && !arg.At.Equal(r.now) {
			return nil, r.boom
		}
		return r.newest()
	}
	return r.close(arg.At), nil
}

func (r dayBaseFake) FirstSamplesSince(
	context.Context,
	sqlc.FirstSamplesSinceParams,
) ([]sqlc.FirstSamplesSinceRow, error) {
	if r.fail {
		return nil, r.boom
	}
	return r.first, nil
}

func TestChart_preIPODayChartMeasuresFromTheFirstSampleOfTheDay(t *testing.T) {
	t.Parallel()
	pre := appleRow(t, "pre_ipo")
	when := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	read := chartRead{
		asset: func() (sqlc.Asset, error) { return pre, nil },
		newest: func() ([]sqlc.NewestSamplesRow, error) {
			return []sqlc.NewestSamplesRow{{Mint: pre.Mint, Ts: when.Add(-time.Minute), PriceMicros: 110_000_000}}, nil
		},
		buckets: func() ([]sqlc.ChartBucketsRow, error) {
			return []sqlc.ChartBucketsRow{
				{Bucket: when, OpenMicros: 1, HighMicros: 1, LowMicros: 1, CloseMicros: 1},
			}, nil
		},
		first: []sqlc.FirstSamplesSinceRow{{Mint: pre.Mint, Ts: when.Add(-time.Hour), PriceMicros: 100_000_000}},
	}
	day, err := (&AssetChart{read: read, base: read, clock: testkit.NewClock(when)}).Handle(t.Context(), "AAPLx", "1D")
	if err != nil || day.PreviousClose == nil || *day.PreviousClose != money.MicrosFromUint64(100_000_000) {
		t.Fatalf("pre-IPO previous close = %v err %v", day.PreviousClose, err)
	}
}
