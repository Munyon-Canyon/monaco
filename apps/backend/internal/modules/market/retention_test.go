package market_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type priceRow struct {
	mint   string
	ts     time.Time
	micros int64
	source string
}

func (r priceRow) String() string {
	return fmt.Sprintf("%s %s %d %s", r.mint, r.ts.UTC().Format(time.RFC3339), r.micros, r.source)
}

func newRetention(t *testing.T) (*pgxpool.Pool, *app.Retention, time.Time) {
	t.Helper()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Hour)
	clk := testkit.NewClock(now)
	return pool, app.NewRetention(db.New(pool, testkit.NewIDs(559), clk), clk), now
}

func insertPrices(t *testing.T, pool *pgxpool.Pool, rows []priceRow) {
	t.Helper()
	for _, row := range rows {
		_, err := pool.Exec(t.Context(),
			`INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, $3, $4)`,
			row.mint, row.ts, row.micros, row.source)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func loadPrices(t *testing.T, pool *pgxpool.Pool) []priceRow {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT mint, ts, price_micros, source FROM price_points ORDER BY mint, ts`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []priceRow
	for rows.Next() {
		var row priceRow
		if err := rows.Scan(&row.mint, &row.ts, &row.micros, &row.source); err != nil {
			t.Fatal(err)
		}
		row.ts = row.ts.UTC()
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func samePriceRows(got, want []priceRow) bool {
	less := func(a, b priceRow) int {
		if a.mint != b.mint {
			return strings.Compare(a.mint, b.mint)
		}
		return a.ts.UTC().Compare(b.ts.UTC())
	}
	got, want = slices.Clone(got), slices.Clone(want)
	slices.SortFunc(got, less)
	slices.SortFunc(want, less)
	return slices.EqualFunc(got, want, func(a, b priceRow) bool {
		return a.mint == b.mint && a.ts.UTC().Equal(b.ts.UTC()) && a.micros == b.micros && a.source == b.source
	})
}

func tickRetention(t *testing.T, p *app.Retention) poller.Report {
	t.Helper()
	report, err := p.Tick(t.Context())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	return report
}

func TestRetention_KeepsRecent(t *testing.T) {
	t.Parallel()
	pool, p, now := newRetention(t)
	if p.Name() != "market.retention" || p.Interval() != 30*24*time.Hour {
		t.Fatalf("poller %s every %s, want market.retention every 720h", p.Name(), p.Interval())
	}
	aapl := marketfake.AAPLx().Mint.String()
	old := now.Add(-10 * 24 * time.Hour)
	want := []priceRow{
		{aapl, now.Add(-6 * 24 * time.Hour), 3, "jupiter"},
		{aapl, now.Add(-7*24*time.Hour + time.Minute), 4, "coingecko"},
		{aapl, now.Add(-time.Hour), 1, "jupiter"},
		{aapl, now.Add(-time.Hour + time.Minute), 2, "coingecko"},
		{aapl, old, 8, "jupiter"},
	}
	insertPrices(t, pool, append(slices.Clone(want), priceRow{aapl, old.Add(time.Minute), 9, "coingecko"}))
	if got := tickRetention(t, p); got.Changed != 1 || got.Scanned != 1 {
		t.Fatalf("report = %+v, want one deleted row", got)
	}
	if got := loadPrices(t, pool); !samePriceRows(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

func TestRetention_FiveMinuteWindow(t *testing.T) {
	t.Parallel()
	pool, p, now := newRetention(t)
	aapl, tsla := marketfake.AAPLx().Mint.String(), marketfake.TSLAx().Mint.String()
	age, recent := now.Add(-10*24*time.Hour), now.Add(-2*time.Hour)
	want := []priceRow{
		{aapl, age, 10, "jupiter"},
		{aapl, age.Add(5 * time.Minute), 13, "jupiter"},
		{aapl, age.Add(10 * time.Minute), 14, "coingecko"},
		{aapl, recent, 30, "jupiter"},
		{aapl, recent.Add(time.Minute), 31, "jupiter"},
		{aapl, recent.Add(2 * time.Minute), 32, "coingecko"},
		{tsla, age.Add(time.Minute), 20, "coingecko"},
		{tsla, recent, 22, "jupiter"},
	}
	insertPrices(t, pool, append(slices.Clone(want),
		priceRow{aapl, age.Add(time.Minute), 11, "jupiter"},
		priceRow{aapl, age.Add(2 * time.Minute), 12, "coingecko"},
		priceRow{aapl, age.Add(11 * time.Minute), 15, "jupiter"},
		priceRow{tsla, age.Add(3 * time.Minute), 21, "jupiter"},
	))
	tickRetention(t, p)
	if got := loadPrices(t, pool); !samePriceRows(got, want) {
		t.Fatalf("rows = %v, want one row per mint per 5 minutes %v", got, want)
	}
}

func TestRetention_Hourly(t *testing.T) {
	t.Parallel()
	pool, p, now := newRetention(t)
	aapl, tsla := marketfake.AAPLx().Mint.String(), marketfake.TSLAx().Mint.String()
	ancient, mid := now.Add(-100*24*time.Hour), now.Add(-10*24*time.Hour)
	want := []priceRow{
		{aapl, ancient, 1, "jupiter"},
		{aapl, ancient.Add(time.Hour), 4, "jupiter"},
		{aapl, mid, 8, "jupiter"},
		{aapl, mid.Add(10 * time.Minute), 9, "coingecko"},
		{aapl, now.Add(-time.Hour), 10, "jupiter"},
		{aapl, now.Add(-time.Hour + time.Minute), 11, "jupiter"},
		{tsla, ancient.Add(5 * time.Minute), 6, "coingecko"},
		{tsla, now.Add(-time.Hour), 12, "jupiter"},
	}
	insertPrices(t, pool, append(slices.Clone(want),
		priceRow{aapl, ancient.Add(10 * time.Minute), 2, "jupiter"},
		priceRow{aapl, ancient.Add(20 * time.Minute), 3, "coingecko"},
		priceRow{aapl, ancient.Add(time.Hour + 30*time.Minute), 5, "jupiter"},
		priceRow{tsla, ancient.Add(50 * time.Minute), 7, "jupiter"},
	))
	if got := tickRetention(t, p); got.Changed != 4 {
		t.Fatalf("Changed = %d, want the four rows inside an older hour", got.Changed)
	}
	if got := loadPrices(t, pool); !samePriceRows(got, want) {
		t.Fatalf("rows = %v, want one row per mint per hour past 90 days %v", got, want)
	}
}

func TestRetention_Idempotent(t *testing.T) {
	t.Parallel()
	pool, p, now := newRetention(t)
	aapl := marketfake.AAPLx().Mint.String()
	old := now.Add(-10 * 24 * time.Hour)
	insertPrices(t, pool, []priceRow{
		{aapl, old, 1, "jupiter"},
		{aapl, old.Add(time.Minute), 2, "coingecko"},
		{aapl, now.Add(-time.Hour), 3, "jupiter"},
	})
	if got := tickRetention(t, p); got.Changed != 1 {
		t.Fatalf("first Changed = %d, want 1", got.Changed)
	}
	left := loadPrices(t, pool)
	if got := tickRetention(t, p); got.Changed != 0 || got.Scanned != 0 {
		t.Fatalf("second report = %+v, want nothing deleted", got)
	}
	if again := loadPrices(t, pool); !samePriceRows(again, left) {
		t.Fatalf("second run rows = %v, want %v", again, left)
	}
}

func TestRetention_EarliestSurvives(t *testing.T) {
	t.Parallel()
	pool, p, now := newRetention(t)
	aapl := marketfake.AAPLx().Mint.String()
	five, hour := now.Add(-10*24*time.Hour), now.Add(-100*24*time.Hour)
	want := []priceRow{
		{aapl, five, 111, "coingecko"},
		{aapl, five.Add(2 * time.Minute), 222, "jupiter"},
		{aapl, hour.Add(5 * time.Minute), 333, "jupiter"},
	}
	insertPrices(t, pool, append(slices.Clone(want),
		priceRow{aapl, hour.Add(40 * time.Minute), 444, "coingecko"},
	))
	tickRetention(t, p)
	if got := loadPrices(t, pool); !samePriceRows(got, want) {
		t.Fatalf("rows = %v, want the earliest sample in each bucket %v", got, want)
	}
}

func TestRetention_KeepsEachMintsNewestRow(t *testing.T) {
	t.Parallel()
	pool, p, now := newRetention(t)
	idle, ancient := marketfake.AAPLx().Mint.String(), marketfake.TSLAx().Mint.String()
	ten, hundred := now.Add(-10*24*time.Hour), now.Add(-100*24*time.Hour)
	want := []priceRow{
		{idle, ten, 1, "jupiter"},
		{idle, ten.Add(time.Minute), 2, "coingecko"},
		{ancient, hundred, 3, "jupiter"},
		{ancient, hundred.Add(30 * time.Minute), 4, "coingecko"},
	}
	insertPrices(t, pool, want)
	tickRetention(t, p)
	if got := loadPrices(t, pool); !samePriceRows(got, want) {
		t.Fatalf("rows = %v, want each mint's newest row after both passes %v", got, want)
	}
}

func TestRetention_cancelledTickFails(t *testing.T) {
	t.Parallel()
	_, p, _ := newRetention(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := p.Tick(ctx); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Tick = %v, want db_unavailable", err)
	}
}
