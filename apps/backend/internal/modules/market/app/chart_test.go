package app

import (
	"context"
	"database/sql"
	"math"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type chartRead struct {
	asset    func() (sqlc.Asset, error)
	newest   func() ([]sqlc.NewestSamplesRow, error)
	earliest func() (time.Time, error)
	buckets  func() ([]sqlc.ChartBucketsPageRow, error)
	got      *sqlc.ChartBucketsPageParams
	older    func(time.Time) bool
	olderErr error
	newestAt func(time.Time) []sqlc.NewestSamplesRow
	first    []sqlc.FirstSamplesSinceRow
}

func (r chartRead) FirstSamplesSince(
	context.Context, sqlc.FirstSamplesSinceParams,
) ([]sqlc.FirstSamplesSinceRow, error) {
	return r.first, nil
}

func (r chartRead) AssetBySymbol(context.Context, string) (sqlc.Asset, error) { return r.asset() }

func (r chartRead) NewestSamples(_ context.Context, arg sqlc.NewestSamplesParams) ([]sqlc.NewestSamplesRow, error) {
	if r.newestAt != nil {
		return r.newestAt(arg.At), nil
	}
	if r.newest == nil {
		return nil, nil
	}
	return r.newest()
}

func (r chartRead) EarliestPrice(context.Context, string) (time.Time, error) { return r.earliest() }

func (r chartRead) HasSampleBefore(_ context.Context, arg sqlc.HasSampleBeforeParams) (bool, error) {
	return r.older != nil && r.older(arg.Before), r.olderErr
}

func (r chartRead) ChartBucketsPage(
	_ context.Context, arg sqlc.ChartBucketsPageParams,
) ([]sqlc.ChartBucketsPageRow, error) {
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
	var got sqlc.ChartBucketsPageParams
	read := chartRead{
		asset: func() (sqlc.Asset, error) { return apple, nil },
		newest: func() ([]sqlc.NewestSamplesRow, error) {
			return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: acceptedAt, PriceMicros: 100}}, nil
		},
		buckets: func() ([]sqlc.ChartBucketsPageRow, error) {
			return []sqlc.ChartBucketsPageRow{{
				Bucket: acceptedAt, OpenMicros: 1, HighMicros: 1, LowMicros: 1, CloseMicros: 1,
			}}, nil
		},
		got: &got,
	}
	_, err := (&AssetChart{read: read, base: read, clock: testkit.NewClock(when)}).Handle(
		t.Context(),
		"AAPLx",
		ChartRequest{Range: "1D"},
	)
	if err != nil || !got.Until.Equal(acceptedAt) || got.Since.Equal(when) {
		t.Fatalf("until = %s since %s err %v", got.Until, got.Since, err)
	}
}

func TestChart_dayChartIsTheLastSessionWhenTheMarketIsClosed(t *testing.T) {
	t.Parallel()
	apple := appleRow(t, "equity")
	saturday := time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)
	fridayOpen := time.Date(2026, 3, 6, 14, 30, 0, 0, time.UTC)
	var got sqlc.ChartBucketsPageParams
	read := chartRead{
		asset: func() (sqlc.Asset, error) { return apple, nil },
		newest: func() ([]sqlc.NewestSamplesRow, error) {
			return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: saturday.Add(-time.Minute), PriceMicros: 100}}, nil
		},
		buckets: func() ([]sqlc.ChartBucketsPageRow, error) { return nil, nil },
		got:     &got,
	}
	chart := &AssetChart{read: read, base: read, clock: testkit.NewClock(saturday)}
	_, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1D"})
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
	bar := sqlc.ChartBucketsPageRow{Bucket: when, OpenMicros: 1, HighMicros: 4, LowMicros: 1, CloseMicros: 3}
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
		{
			raw: "1D",
			read: chartRead{asset: okAsset, newest: sample, buckets: func() ([]sqlc.ChartBucketsPageRow, error) {
				return nil, boom
			}},
			code: errs.CodeInternal,
		},
		{
			raw: "1D",
			read: chartRead{asset: okAsset, newest: sample, buckets: func() ([]sqlc.ChartBucketsPageRow, error) {
				bad := sqlc.ChartBucketsPageRow{
					Bucket:      when,
					OpenMicros:  1,
					HighMicros:  1,
					LowMicros:   1,
					CloseMicros: -1,
				}
				return []sqlc.ChartBucketsPageRow{bad}, nil
			}},
			code: errs.CodeDecodeFailed,
		},
		{raw: "1D", read: chartRead{asset: okAsset, newest: func() ([]sqlc.NewestSamplesRow, error) {
			return []sqlc.NewestSamplesRow{{Ts: when.Add(-400 * 24 * time.Hour), PriceMicros: 50}}, nil
		}, buckets: func() ([]sqlc.ChartBucketsPageRow, error) { return nil, nil }}, empty: true},
		{raw: "1D", read: chartRead{asset: okAsset, newest: func() ([]sqlc.NewestSamplesRow, error) {
			return []sqlc.NewestSamplesRow{
				{Ts: when.Add(-time.Hour), PriceMicros: 50},
				{Ts: when, PriceMicros: 400},
			}, nil
		}, buckets: func() ([]sqlc.ChartBucketsPageRow, error) { return []sqlc.ChartBucketsPageRow{bar}, nil }}},
	}
	for i, tc := range cases {
		chart := &AssetChart{read: tc.read, base: tc.read, clock: testkit.NewClock(when)}
		got, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: tc.raw})
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
		buckets: func() ([]sqlc.ChartBucketsPageRow, error) {
			return []sqlc.ChartBucketsPageRow{{
				Bucket: when.Add(-time.Minute), OpenMicros: 1, HighMicros: 1, LowMicros: 1, CloseMicros: 1,
			}}, nil
		},
	}
	chart := &AssetChart{read: read, base: read, clock: testkit.NewClock(when)}
	day, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1D"})
	if err != nil || day.PreviousClose == nil || *day.PreviousClose != money.MicrosFromUint64(98_000_000) {
		t.Fatalf("1D previous close = %v err %v", day.PreviousClose, err)
	}
	for _, raw := range []string{"1W", "1M", "3M", "1Y"} {
		other, otherErr := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: raw})
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
		buckets: func() ([]sqlc.ChartBucketsPageRow, error) {
			return []sqlc.ChartBucketsPageRow{{
				Bucket: when.Add(-time.Minute), OpenMicros: 1, HighMicros: 1, LowMicros: 1, CloseMicros: 1,
			}}, nil
		},
	}
	day, err := (&AssetChart{read: read, base: read, clock: testkit.NewClock(when)}).Handle(
		t.Context(),
		"AAPLx",
		ChartRequest{Range: "1D"},
	)
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
			buckets: func() ([]sqlc.ChartBucketsPageRow, error) {
				return []sqlc.ChartBucketsPageRow{
					{Bucket: tc.at, OpenMicros: 1, HighMicros: 1, LowMicros: 1, CloseMicros: 1},
				}, nil
			},
		}, close: tc.close, first: tc.first, boom: boom, now: tc.at, fail: tc.fail}
		chart := &AssetChart{read: read, base: read, clock: testkit.NewClock(tc.at)}
		if _, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1D"}); err == nil {
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
		buckets: func() ([]sqlc.ChartBucketsPageRow, error) {
			return []sqlc.ChartBucketsPageRow{
				{Bucket: when, OpenMicros: 1, HighMicros: 1, LowMicros: 1, CloseMicros: 1},
			}, nil
		},
		first: []sqlc.FirstSamplesSinceRow{{Mint: pre.Mint, Ts: when.Add(-time.Hour), PriceMicros: 100_000_000}},
	}
	day, err := (&AssetChart{read: read, base: read, clock: testkit.NewClock(when)}).Handle(
		t.Context(),
		"AAPLx",
		ChartRequest{Range: "1D"},
	)
	if err != nil || day.PreviousClose == nil || *day.PreviousClose != money.MicrosFromUint64(100_000_000) {
		t.Fatalf("pre-IPO previous close = %v err %v", day.PreviousClose, err)
	}
}

func pagedChart(
	t *testing.T, older func(time.Time) bool, buckets int,
) (*AssetChart, *sqlc.ChartBucketsPageParams, time.Time) {
	t.Helper()
	apple := appleRow(t, "equity")
	when := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	var got sqlc.ChartBucketsPageParams
	read := chartRead{
		asset: func() (sqlc.Asset, error) { return apple, nil },
		newest: func() ([]sqlc.NewestSamplesRow, error) {
			return []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: when.Add(-time.Minute), PriceMicros: 100}}, nil
		},
		earliest: func() (time.Time, error) { return when.Add(-300 * 24 * time.Hour), nil },
		buckets: func() ([]sqlc.ChartBucketsPageRow, error) {
			rows := make([]sqlc.ChartBucketsPageRow, 0, buckets)
			for i := range buckets {
				rows = append(rows, sqlc.ChartBucketsPageRow{
					Bucket: when.Add(-time.Duration(buckets-i) * time.Hour), OpenMicros: 1, HighMicros: 1, LowMicros: 1,
					CloseMicros: 1,
				})
			}
			return rows, nil
		},
		older: older,
		got:   &got,
	}
	return &AssetChart{read: read, base: read, clock: testkit.NewClock(when)}, &got, when
}

func TestChart_defaultIntervalIsTodaysBucketForEveryRange(t *testing.T) {
	t.Parallel()
	want := map[string]time.Duration{
		"1D": 5 * time.Minute, "1W": time.Hour, "1M": time.Hour, "3M": time.Hour,
		"6M": 24 * time.Hour, "1Y": 24 * time.Hour, "ALL": 24 * time.Hour,
	}
	for raw, width := range want {
		chart, got, _ := pagedChart(t, nil, 1)
		out, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: raw})
		if err != nil || out.Bucket != width || got.BucketSeconds != int64(width/time.Second) ||
			out.Interval != domain.ChartRange(raw).DefaultInterval() {
			t.Fatalf("%s = %+v %+v %v", raw, out, got, err)
		}
	}
}

func TestChart_anExplicitIntervalSetsTheBucketWidth(t *testing.T) {
	t.Parallel()
	chart, got, _ := pagedChart(t, nil, 1)
	out, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1W", Interval: "15m"})
	if err != nil || out.Bucket != 15*time.Minute || out.Interval != domain.Interval15m || got.BucketSeconds != 900 {
		t.Fatalf("15m = %+v %+v %v", out, got, err)
	}
}

func TestChart_rejectsAnIntervalOrLimitOutsideTheContract(t *testing.T) {
	t.Parallel()
	for _, req := range []ChartRequest{
		{Range: "1Y", Interval: "5m"},
		{Range: "6M", Interval: "30m"},
		{Range: "1D", Interval: "1m"},
		{Range: "1D", Interval: "15M"},
		{Range: "1M", Interval: "1w"},
		{Range: "1D", Limit: 1001},
		{Range: "1D", Limit: -1},
	} {
		chart, _, _ := pagedChart(t, nil, 1)
		if _, err := chart.Handle(t.Context(), "AAPLx", req); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("%+v = %v", req, err)
		}
	}
}

func TestChart_limitIsUncappedOnTheFirstPageAndFiveHundredOnABeforePage(t *testing.T) {
	t.Parallel()
	chart, got, when := pagedChart(t, nil, 1)
	if _, err := chart.Handle(
		t.Context(),
		"AAPLx",
		ChartRequest{Range: "1M"},
	); err != nil ||
		got.PageLimit != math.MaxInt32 {
		t.Fatalf("first page limit = %d %v, want uncapped", got.PageLimit, err)
	}
	before := when.Add(-time.Hour)
	if _, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1D", Before: &before}); err != nil ||
		got.PageLimit != 500 {
		t.Fatalf("before page limit = %d %v", got.PageLimit, err)
	}
	for _, req := range []ChartRequest{{Range: "1D", Limit: 1000}, {Range: "1D", Limit: 7, Before: &before}} {
		if _, err := chart.Handle(t.Context(), "AAPLx", req); err != nil || int(got.PageLimit) != req.Limit {
			t.Fatalf("%+v limit = %d %v", req, got.PageLimit, err)
		}
	}
}

func TestChart_beforeAlignsDownReadsToTheFirstSampleAndStopsBeforeIt(t *testing.T) {
	t.Parallel()
	chart, got, when := pagedChart(t, nil, 1)
	before := time.Date(2026, 3, 4, 12, 7, 31, 0, time.UTC)
	out, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1D", Interval: "15m", Before: &before})
	aligned := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	if err != nil || !got.Until.Equal(aligned.Add(-time.Microsecond)) {
		t.Fatalf("until = %s err %v, want %s", got.Until, err, aligned.Add(-time.Microsecond))
	}
	if !got.Since.Equal(when.Add(-300 * 24 * time.Hour)) {
		t.Fatalf("since = %s, want the earliest sample, not the range start", got.Since)
	}
	if out.PreviousClose != nil {
		t.Fatalf("a before page carries the day-change base: %v", out.PreviousClose)
	}
}

func TestChart_beforeLaterThanTheNewestSampleKeepsTheNewestAsTheCeiling(t *testing.T) {
	t.Parallel()
	chart, got, when := pagedChart(t, nil, 1)
	before := when.Add(time.Hour)
	if _, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1D", Before: &before}); err != nil {
		t.Fatal(err)
	}
	if !got.Until.Equal(when.Add(-time.Minute)) {
		t.Fatalf("until = %s, want the accepted sample", got.Until)
	}
}

func TestChart_nextBeforeIsSetOnlyWhenAnOlderSampleExists(t *testing.T) {
	t.Parallel()
	for _, hasOlder := range []bool{true, false} {
		var asked time.Time
		chart, _, when := pagedChart(t, func(at time.Time) bool { asked = at; return hasOlder }, 3)
		out, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1W"})
		oldest := when.Add(-3 * time.Hour)
		if err != nil || !asked.Equal(oldest) {
			t.Fatalf("asked for samples before %s, want %s (%v)", asked, oldest, err)
		}
		if hasOlder && (out.NextBefore == nil || !out.NextBefore.Equal(oldest)) {
			t.Fatalf("next_before = %v, want %s", out.NextBefore, oldest)
		}
		if !hasOlder && out.NextBefore != nil {
			t.Fatalf("next_before = %v, want nil", out.NextBefore)
		}
	}
}

func TestChart_aFailedOlderSampleLookupFailsTheChart(t *testing.T) {
	t.Parallel()
	chart, _, _ := pagedChart(t, func(time.Time) bool { return true }, 3)
	read, ok := chart.read.(chartRead)
	if !ok {
		t.Fatalf("read is %T", chart.read)
	}
	read.olderErr = errs.New(errs.CodeInternal, "market.test")
	chart.read = read
	if _, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1W"}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v, want %s", err, errs.CodeInternal)
	}
}

func TestChart_anEmptyPageHasNoNextBefore(t *testing.T) {
	t.Parallel()
	chart, _, _ := pagedChart(t, func(time.Time) bool { return true }, 0)
	out, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1D"})
	if err != nil || !out.Empty || out.NextBefore != nil {
		t.Fatalf("empty page = %+v %v", out, err)
	}
}

func TestAlignDown_snapsToTheBinOrigin(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 3, 4, 12, 7, 31, 500, time.UTC)
	cases := map[time.Duration]time.Time{
		5 * time.Minute:    time.Date(2026, 3, 4, 12, 5, 0, 0, time.UTC),
		15 * time.Minute:   time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC),
		4 * time.Hour:      time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC),
		24 * time.Hour:     time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC),
		7 * 24 * time.Hour: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
	}
	for width, want := range cases {
		if got := alignDown(at, width); !got.Equal(want) {
			t.Fatalf("%s = %s, want %s", width, got, want)
		}
	}
}
