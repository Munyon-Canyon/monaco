package app

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func yearNow() time.Time { return time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC) }

func yearMint(t *testing.T) domain.Mint {
	t.Helper()
	mint, err := domain.ParseMint(rangeMint)
	if err != nil {
		t.Fatal(err)
	}
	return mint
}

func dailyHistory(days int, low, high uint64) []Sample {
	out := make([]Sample, 0, days+1)
	for i := days; i >= 0; i-- {
		price := low + uint64(i%7)*(high-low)/6
		out = append(out, Sample{At: yearNow().AddDate(0, 0, -i), Price: usd(price)})
	}
	return out
}

func seedDays(t *testing.T, pool sqlc.DBTX, mint domain.Mint, first, last int, low, high int64) {
	t.Helper()
	for ago := first; ago <= last; ago++ {
		_, err := pool.Exec(t.Context(),
			`INSERT INTO price_day_ranges (mint, day, high_micros, low_micros, source)
			VALUES ($1, $2::date, $3, $4, 'samples')`,
			mint.String(), yearNow().AddDate(0, 0, -ago).Format(time.DateOnly), high, low)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func rangeOf(t *testing.T, ranges *YearRanges) (low, high *money.Micros) {
	t.Helper()
	low, high, err := ranges.YearRange(t.Context(), yearMint(t), yearNow())
	if err != nil {
		t.Fatal(err)
	}
	return low, high
}

func TestYearRange_aYearOfSampleDaysGivesTheLowestLowAndHighestHigh(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedDays(t, pool, yearMint(t), 1, 363, 100_000_000, 150_000_000)
	seedDays(t, pool, yearMint(t), 364, 364, 86_620_000, 224_900_000)
	low, high := rangeOf(t, NewYearRanges(pool))
	if low == nil || high == nil || low.Uint64() != 86_620_000 || high.Uint64() != 224_900_000 {
		t.Fatalf("range = %v..%v, want 86.62..224.90", low, high)
	}
}

func TestYearRange_aDayOlderThanTheYearIsLeftOut(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedDays(t, pool, yearMint(t), 1, 364, 100_000_000, 150_000_000)
	seedDays(t, pool, yearMint(t), 366, 366, 1_000_000, 900_000_000)
	low, high := rangeOf(t, NewYearRanges(pool))
	if low == nil || high == nil || low.Uint64() != 100_000_000 || high.Uint64() != 150_000_000 {
		t.Fatalf("range = %v..%v, want the 366-day-old day ignored", low, high)
	}
}

func TestYearRange_aStockListedThirtyDaysAgoHasNoRange(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedDays(t, pool, yearMint(t), 1, 30, 100_000_000, 150_000_000)
	for _, hist := range []PriceHistory{nil, &fakeHistory{samples: dailyHistory(30, 100, 150)}, &fakeHistory{off: true}} {
		ranges := NewYearRanges(pool)
		if hist != nil {
			ranges.WithHistory(hist)
		}
		if low, high := rangeOf(t, ranges); low != nil || high != nil {
			t.Fatalf("range = %v..%v with history %T, want null for both", low, high, hist)
		}
	}
}

func rowsBySource(t *testing.T, pool sqlc.DBTX) (count map[string]int, samplesTop int64) {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT source, count(*), max(high_micros) FROM price_day_ranges GROUP BY source`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count = map[string]int{}
	for rows.Next() {
		var source string
		var n int
		var top int64
		if err := rows.Scan(&source, &n, &top); err != nil {
			t.Fatal(err)
		}
		count[source] = n
		if source == "samples" {
			samplesTop = top
		}
	}
	return count, samplesTop
}

func TestYearRange_theHistoryFillsTheDaysBeforeSamplesAndASamplesDayWins(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	mint := yearMint(t)
	seedDays(t, pool, mint, 0, 2, 50_000_000, 300_000_000)
	hist := &fakeHistory{samples: dailyHistory(365, 100, 200)}
	ranges := NewYearRanges(pool).WithHistory(hist)

	low, high := rangeOf(t, ranges)
	if low == nil || high == nil || low.Uint64() != 50_000_000 || high.Uint64() != 300_000_000 {
		t.Fatalf("range = %v..%v, want the samples days' 50..300 over the history's 100..200", low, high)
	}
	count, samplesTop := rowsBySource(t, pool)
	if count["samples"] != 3 || count["history"] != 362 || samplesTop != 300_000_000 {
		t.Fatalf("rows by source = %v, samples top %d, want 3 samples days kept at 300e6 and 362 history days",
			count, samplesTop)
	}

	rangeOf(t, ranges)
	if calls := hist.calls(); len(calls) != 1 || calls[0] != 365 {
		t.Fatalf("history calls = %v, want one 365-day read, the same series the 1Y chart loads", calls)
	}
}

func TestYearRange_aFailingHistoryLeavesTheRangeNull(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	hist := &fakeHistory{err: context.DeadlineExceeded}
	if low, high := rangeOf(t, NewYearRanges(pool).WithHistory(hist)); low != nil || high != nil {
		t.Fatalf("range = %v..%v, want null", low, high)
	}
}

func TestYearRange_todaysRawAcceptedSamplesCountAndRejectedOnesDoNot(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	mint := yearMint(t)
	seedDays(t, pool, mint, 1, 364, 100_000_000, 130_000_000)
	for i, micros := range []int64{120_000_000, 140_000_000, 700_000_000} {
		_, err := pool.Exec(t.Context(),
			`INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, $3, 'jupiter')`,
			mint.String(), yearNow().Add(-time.Duration(3-i)*time.Hour), micros)
		if err != nil {
			t.Fatal(err)
		}
	}
	low, high := rangeOf(t, NewYearRanges(pool))
	if low == nil || high == nil || low.Uint64() != 100_000_000 || high.Uint64() != 140_000_000 {
		t.Fatalf("range = %v..%v, want today's 140e6 in and the 700e6 spike out", low, high)
	}
}

type yearReader struct {
	rows      []sqlc.YearRangeRow
	errs      []error
	upsertErr error
	reads     int
	upserts   int
}

func (r *yearReader) YearRange(context.Context, sqlc.YearRangeParams) (sqlc.YearRangeRow, error) {
	i := r.reads
	r.reads++
	return r.rows[i], r.errs[i]
}

func (r *yearReader) UpsertHistoryDayRanges(context.Context, sqlc.UpsertHistoryDayRangesParams) error {
	r.upserts++
	return r.upsertErr
}

func TestYearRange_aFailingReadFailsTheCallAndTheDetail(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "market.test")
	reader := &yearReader{rows: make([]sqlc.YearRangeRow, 1), errs: []error{boom}}
	ranges := &YearRanges{read: reader}
	if _, _, err := ranges.YearRange(t.Context(), yearMint(t), yearNow()); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
	detail := &Detail{facts: statsFacts{}, years: ranges}
	reader.reads = 0
	item := Summary{Asset: domain.Asset{Mint: yearMint(t)}}
	if _, err := detail.stats(t.Context(), item, yearNow()); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("stats err = %v, want internal", err)
	}
}

type statsFacts struct{}

func (statsFacts) MarketStats(context.Context, sqlc.MarketStatsParams) (sqlc.MarketStatsRow, error) {
	return sqlc.MarketStatsRow{}, nil
}

func TestYearRange_aFailedHistoryWriteOrSecondReadLeavesNoRangeOrFailsTheCall(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "market.test")
	hist := &fakeHistory{samples: dailyHistory(365, 100, 200)}
	uncovered := sqlc.YearRangeRow{OldestGapDays: 30, LowMicros: 1, HighMicros: 2}

	write := &yearReader{rows: []sqlc.YearRangeRow{uncovered}, errs: []error{nil}, upsertErr: boom}
	low, high, err := (&YearRanges{read: write, hist: hist}).YearRange(t.Context(), yearMint(t), yearNow())
	if low != nil || high != nil || err != nil || write.upserts != 1 || write.reads != 1 {
		t.Fatalf("failed write = %v..%v, %v after %d upserts and %d reads, want null and no second read",
			low, high, err, write.upserts, write.reads)
	}

	reread := &yearReader{rows: []sqlc.YearRangeRow{uncovered, {}}, errs: []error{nil, boom}}
	_, _, err = (&YearRanges{read: reread, hist: hist}).YearRange(t.Context(), yearMint(t), yearNow())
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("second read err = %v, want internal", err)
	}
}

func TestYearRange_historyThatHoldsNoUsableDayWritesNothing(t *testing.T) {
	t.Parallel()
	old := []Sample{{At: yearNow().AddDate(-2, 0, 0), Price: usd(100)}, {At: yearNow(), Price: usd(0)}}
	reader := &yearReader{rows: []sqlc.YearRangeRow{{}}, errs: []error{nil}}
	ranges := &YearRanges{read: reader, hist: &fakeHistory{samples: old}}
	low, high, err := ranges.YearRange(t.Context(), yearMint(t), yearNow())
	if low != nil || high != nil || err != nil || reader.upserts != 0 {
		t.Fatalf("range = %v..%v, %v after %d upserts, want null and no write", low, high, err, reader.upserts)
	}
}
