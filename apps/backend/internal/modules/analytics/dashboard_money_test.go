package analytics_test

import (
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/analyticsapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func at(day, hour, minute int) time.Time {
	return time.Date(2026, 9, day, hour, minute, 0, 0, time.UTC)
}

func seedMoney(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	s := ledgerSeed{t: t, pool: pool, ids: testkit.NewIDs(1)}
	s.fundMember(at(1, 10, 0), 100_000_000)
	s.swap("settled", at(1, 23, 30), 40_000_000)
	s.fundMember(at(3, 12, 0), 50_000_000)
	s.swap("settled", at(3, 12, 30), -15_000_000)
	s.cashOut("settled", at(7, 0, 0), 20_000_000)
	s.wallet("deposit", "settled", at(8, 9, 0), 30_000_000)
	s.wallet("withdrawal", "settled", at(8, 18, 0), -10_000_000)
	s.wallet("withdrawal", "pending", at(2, 9, 0), -5_000_000)
	s.swap("failed", at(2, 9, 0), 99_000_000)
	s.swap("settled", time.Date(2026, 8, 31, 23, 59, 59, 0, time.UTC), 98_000_000)
	s.swap("settled", at(15, 0, 0), 97_000_000)
	e := signalSeed{t: t, pool: pool, ids: testkit.NewIDs(9)}
	e.event("onramp.status_changed", at(1, 11, 0), map[string]any{"to": "completed"})
	e.event("onramp.status_changed", at(1, 12, 0), map[string]any{"to": "completed"})
	e.event("onramp.status_changed", at(3, 12, 0), map[string]any{"to": "failed"})
}

func seedValuation(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	asOf := at(14, 12, 0)
	if _, err := pool.Exec(t.Context(), `INSERT INTO leaderboard_runs
		(run_id, as_of, prices_as_of, started_at, finished_at, rows_written, cabals_excluded)
		VALUES ($1, $2, $2, $2, $2, 2, 0)`, testkit.NewIDs(2).NewV7(), asOf); err != nil {
		t.Fatal(err)
	}
	gen := testkit.NewIDs(3)
	first, second := gen.NewV7(), gen.NewV7()
	for _, snap := range []struct {
		cabal uuid.UUID
		value int64
		at    time.Time
	}{{first, 60_000_000, asOf.Add(-time.Hour)}, {first, 70_000_000, asOf}, {second, 5_000_000, asOf}} {
		if _, err := pool.Exec(t.Context(), `INSERT INTO cabal_value_snapshots
			(cabal_id, at, value_micros, nav_per_share_micros, total_shares) VALUES ($1, $2, $3, 1, 1)`,
			snap.cabal, snap.at, snap.value); err != nil {
			t.Fatal(err)
		}
	}
}

func starting(start time.Time, b api.MoneyBucket) api.MoneyBucket {
	b.BucketStart = start
	return b
}

func zeros() api.MoneyBucket {
	return api.MoneyBucket{
		DepositMicros: "0", FundMicros: "0", SwapBuyMicros: "0", SwapSellMicros: "0", CashOutMicros: "0",
		WithdrawalMicros: "0",
	}
}

func with(mutate func(*api.MoneyBucket)) api.MoneyBucket {
	b := zeros()
	mutate(&b)
	return b
}

func TestDashboard_Money_Totals(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedMoney(t, pool)
	seedValuation(t, pool)
	h := productHandler(t, pool)
	tests := map[string]struct {
		bucket string
		series [2]time.Time
		want   []api.MoneyBucket
	}{
		"day": {"day", [2]time.Time{at(1, 0, 0), at(3, 0, 0)}, []api.MoneyBucket{
			starting(at(1, 0, 0), with(func(b *api.MoneyBucket) {
				b.DepositCount, b.DepositMicros, b.FundCount, b.FundMicros = 1, "100000000", 1, "100000000"
				b.SwapBuyMicros = "40000000"
			})),
			starting(at(3, 0, 0), with(func(b *api.MoneyBucket) {
				b.DepositCount, b.DepositMicros, b.FundCount, b.FundMicros = 1, "50000000", 1, "50000000"
				b.SwapSellMicros = "15000000"
			})),
			starting(at(7, 0, 0), with(func(b *api.MoneyBucket) { b.CashOutCount, b.CashOutMicros = 1, "20000000" })),
			starting(at(8, 0, 0), with(func(b *api.MoneyBucket) {
				b.DepositCount, b.DepositMicros, b.WithdrawalCount, b.WithdrawalMicros = 1, "30000000", 1, "10000000"
			})),
		}},
		"week": {
			"week",
			[2]time.Time{time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)},
			[]api.MoneyBucket{
				starting(time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), with(func(b *api.MoneyBucket) {
					b.DepositCount, b.DepositMicros, b.FundCount, b.FundMicros = 2, "150000000", 2, "150000000"
					b.SwapBuyMicros, b.SwapSellMicros = "40000000", "15000000"
				})),
				starting(at(7, 0, 0), with(func(b *api.MoneyBucket) {
					b.DepositCount, b.DepositMicros, b.WithdrawalCount, b.WithdrawalMicros = 1, "30000000", 1, "10000000"
					b.CashOutCount, b.CashOutMicros = 1, "20000000"
				})),
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var got api.MoneyDashboard
			decode(t, dashboardGet(t, h, "money", viewerToken, window("2026-09-01", "2026-09-15", tt.bucket)), &got)
			if !slices.Equal(got.Buckets, tt.want) {
				t.Errorf("buckets = %+v, want %+v", got.Buckets, tt.want)
			}
			wantSeries := []api.DashboardPoint{
				{BucketStart: tt.series[0], Metric: "onramp_status_changes", Group: "completed", Count: 2},
				{BucketStart: tt.series[1], Metric: "onramp_status_changes", Group: "failed", Count: 1},
			}
			if !slices.Equal(got.Series, wantSeries) {
				t.Errorf("series = %+v, want %+v", got.Series, wantSeries)
			}
			assertMoneyTotals(t, got, tt.bucket)
		})
	}
}

func assertMoneyTotals(t *testing.T, got api.MoneyDashboard, bucket string) {
	t.Helper()
	wantTotals := [4]string{"75000000", "20000000", "95000000", at(14, 12, 0).String()}
	gotTotals := [4]string{got.PotMicros, got.PlatformBalanceMicros, got.TotalValueHeldMicros, ""}
	if got.AsOf != nil {
		gotTotals[3] = got.AsOf.String()
	}
	if gotTotals != wantTotals {
		t.Errorf("pot, platform, total, as_of = %v, want %v", gotTotals, wantTotals)
	}
	if !got.From.Equal(at(1, 0, 0)) || !got.To.Equal(at(15, 0, 0)) || string(got.Bucket) != bucket {
		t.Errorf("window = %v %v %s, want 2026-09-01 2026-09-15 %s", got.From, got.To, got.Bucket, bucket)
	}
}

func TestDashboard_Money_BeforeTheFirstValuation(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	var got api.MoneyDashboard
	decode(t, dashboardGet(t, productHandler(t, pool), "money", viewerToken,
		window("2026-09-01T00:00:00Z", "2026-09-02T00:00:00Z", "day")), &got)
	if got.AsOf != nil || got.PotMicros != "0" || got.TotalValueHeldMicros != "0" || len(got.Buckets) != 0 {
		t.Fatalf("dashboard = %+v, want an empty dashboard with no as_of", got)
	}
}

func TestDashboard_Money_RefusesABadWindow(t *testing.T) {
	t.Parallel()
	h := productHandler(t, testkit.DB(t))
	tests := map[string]url.Values{
		"over 400 days":   window("2025-01-01", "2026-02-07", "day"),
		"to before from":  window("2026-09-02", "2026-09-01", "day"),
		"equal bounds":    window("2026-09-01", "2026-09-01", "week"),
		"not a date":      window("yesterday", "2026-09-01", "day"),
		"unknown bucket":  window("2026-09-01", "2026-09-02", "month"),
		"missing a bound": {"from": {"2026-09-01"}, "bucket": {"day"}},
	}
	for name, q := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := dashboardGet(t, h, "money", viewerToken, q)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d %s, want 400", w.Code, w.Body)
			}
		})
	}
	exactly := dashboardGet(t, h, "money", viewerToken, window("2025-01-01", "2026-02-05", "week"))
	if exactly.Code != http.StatusOK {
		t.Fatalf("400 days: status = %d %s, want 200", exactly.Code, exactly.Body)
	}
	tooLong := dashboardGet(t, h, "money", viewerToken, window("2025-01-01", "2026-02-07", "day"))
	if code := problemCode(t, tooLong); code != string(errs.CodeInvalidInput) {
		t.Fatalf("code = %s, want invalid_input", code)
	}
}

func TestDashboard_Money_RefusesAnAnonymousCaller(t *testing.T) {
	t.Parallel()
	h := productHandler(t, testkit.DB(t))
	w := dashboardGet(t, h, "money", "nobody", window("2026-09-01", "2026-09-02", "day"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestDashboard_Money_QueryCount(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedMoney(t, pool)
	seedValuation(t, pool)
	h := productHandler(t, pool)
	testkit.AssertQueries(t, "analytics GetMoneyDashboard", func() {
		w := dashboardGet(t, h, "money", viewerToken, window("2026-09-01", "2026-09-15", "day"))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d %s", w.Code, w.Body)
		}
	})
}
