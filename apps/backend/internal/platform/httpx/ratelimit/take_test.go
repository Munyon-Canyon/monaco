package ratelimit_test

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/ratelimit"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func epoch() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }

func perMinute() ratelimit.Policy { return ratelimit.Policy{Rate: 10, Per: time.Minute, Burst: 10} }

func newLimiter(t *testing.T, db sqlc.DBTX, clk *testkit.Clock) *ratelimit.Limiter {
	t.Helper()
	l, err := ratelimit.New(db, clk, noop.NewMeterProvider())
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func take(t *testing.T, l *ratelimit.Limiter, key string, p ratelimit.Policy, cost int64) ratelimit.Decision {
	t.Helper()
	d, err := l.Take(t.Context(), key, p, cost)
	if err != nil {
		t.Fatalf("Take(%s) = %v", key, err)
	}
	return d
}

func TestTake_ConcurrentBurst(t *testing.T) {
	t.Parallel()
	l := newLimiter(t, testkit.DB(t), testkit.NewClock(epoch()))
	var allowed, refused atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 100 {
		wg.Go(func() {
			<-start
			d, err := l.Take(t.Context(), "burst", perMinute(), 1)
			switch {
			case err != nil:
				t.Error(err)
			case d.Allowed:
				allowed.Add(1)
			default:
				refused.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()
	if allowed.Load() != 10 || refused.Load() != 90 {
		t.Fatalf("allowed %d refused %d of 100 on burst 10, want 10 and 90", allowed.Load(), refused.Load())
	}
}

func TestTake_Refill(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(epoch())
	l := newLimiter(t, testkit.DB(t), clk)
	for i := range 10 {
		if d := take(t, l, "refill", perMinute(), 1); !d.Allowed {
			t.Fatalf("take %d refused inside the burst", i)
		}
	}
	if d := take(t, l, "refill", perMinute(), 1); d.Allowed || d.RetryAfter != 6*time.Second {
		t.Fatalf("take past the burst = %+v, want refused with RetryAfter 6s", d)
	}
	clk.Advance(perMinute().Per / time.Duration(perMinute().Rate))
	if d := take(t, l, "refill", perMinute(), 1); !d.Allowed {
		t.Fatalf("take after one refill interval = %+v, want allowed", d)
	}
	if d := take(t, l, "refill", perMinute(), 1); d.Allowed {
		t.Fatalf("second take after one refill interval = %+v, want refused", d)
	}
}

func TestTake_RetryAfterIsTheTimeUntilTheDeficitRefills(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(epoch())
	l := newLimiter(t, testkit.DB(t), clk)
	p := ratelimit.Policy{Rate: 3, Per: time.Second, Burst: 4}
	if d := take(t, l, "deficit", p, 4); !d.Allowed {
		t.Fatalf("take of the whole burst = %+v, want allowed", d)
	}
	clk.Advance(100 * time.Millisecond)
	d := take(t, l, "deficit", p, 2)
	if want := 566667 * time.Microsecond; d.Allowed || d.RetryAfter != want {
		t.Fatalf("take of 2 = %+v, want refused with RetryAfter %s", d, want)
	}
	clk.Advance(d.RetryAfter)
	if d := take(t, l, "deficit", p, 2); !d.Allowed {
		t.Fatalf("take of 2 after RetryAfter = %+v, want allowed", d)
	}
}

func TestTake_RefillStopsAtTheBurst(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(epoch())
	l := newLimiter(t, testkit.DB(t), clk)
	take(t, l, "cap", perMinute(), 1)
	clk.Advance(time.Hour)
	if d := take(t, l, "cap", perMinute(), 10); !d.Allowed {
		t.Fatalf("take of the burst after an idle hour = %+v, want allowed", d)
	}
	if d := take(t, l, "cap", perMinute(), 1); d.Allowed {
		t.Fatalf("take past a refilled burst = %+v, want refused", d)
	}
}

func TestTake_ClockBehindTheBucketRefillsNothing(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(epoch())
	l := newLimiter(t, testkit.DB(t), clk)
	take(t, l, "skew", perMinute(), 10)
	clk.Advance(-time.Minute)
	d := take(t, l, "skew", perMinute(), 1)
	if d.Allowed || d.RetryAfter != time.Minute+6*time.Second {
		t.Fatalf("take on a clock a minute behind = %+v, want refused with RetryAfter 1m6s", d)
	}
}

func TestTake_KeysAreIndependent(t *testing.T) {
	t.Parallel()
	l := newLimiter(t, testkit.DB(t), testkit.NewClock(epoch()))
	take(t, l, "a", perMinute(), 10)
	if d := take(t, l, "b", perMinute(), 10); !d.Allowed {
		t.Fatalf("take on a fresh key = %+v, want allowed", d)
	}
}

func TestTake_rejectsAPolicyOrCostItCannotServe(t *testing.T) {
	t.Parallel()
	l := newLimiter(t, nil, testkit.NewClock(epoch()))
	for name, tc := range map[string]struct {
		p    ratelimit.Policy
		cost int64
	}{
		"zero rate":           {ratelimit.Policy{Rate: 0, Per: time.Minute, Burst: 1}, 1},
		"rate over the cap":   {ratelimit.Policy{Rate: 100_001, Per: time.Hour, Burst: 1}, 1},
		"zero burst":          {ratelimit.Policy{Rate: 1, Per: time.Minute, Burst: 0}, 0},
		"burst over the cap":  {ratelimit.Policy{Rate: 100_000, Per: time.Second, Burst: 100_001}, 1},
		"sub-microsecond per": {ratelimit.Policy{Rate: 1, Per: time.Nanosecond, Burst: 1}, 1},
		"per over a day":      {ratelimit.Policy{Rate: 2, Per: 25 * time.Hour, Burst: 1}, 1},
		"refill over a day":   {ratelimit.Policy{Rate: 1, Per: time.Hour, Burst: 25}, 1},
		"zero cost":           {perMinute(), 0},
		"cost over the burst": {perMinute(), 11},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, err := l.Take(t.Context(), "k", tc.p, tc.cost)
			if errs.CodeOf(err) != errs.CodeInvalidConfig || d != (ratelimit.Decision{}) {
				t.Fatalf("Take = %+v, %v, want invalid_config", d, err)
			}
		})
	}
}

func TestTake_acceptsTheLargestPolicy(t *testing.T) {
	t.Parallel()
	l := newLimiter(t, testkit.DB(t), testkit.NewClock(epoch()))
	p := ratelimit.Policy{Rate: 100_000, Per: 24 * time.Hour, Burst: 100_000}
	if d := take(t, l, "max", p, 100_000); !d.Allowed {
		t.Fatalf("take of the largest burst = %+v, want allowed", d)
	}
	if d := take(t, l, "max", p, 1); d.Allowed || d.RetryAfter != 864*time.Millisecond {
		t.Fatalf("take past the largest burst = %+v, want refused with RetryAfter 864ms", d)
	}
}

type failingDB struct {
	take error
	get  error
}

func (f failingDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, f.take
}

func (f failingDB) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, f.take }

func (f failingDB) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	if strings.Contains(sql, "GetRateLimitBucket") {
		return errRow{f.get}
	}
	return errRow{f.take}
}

type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

func TestTake_storeErrorsAreDBUnavailable(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeInternal, "test.down")
	for name, db := range map[string]failingDB{
		"take fails":               {take: down},
		"read after refusal fails": {take: pgx.ErrNoRows, get: down},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, err := newLimiter(t, db, testkit.NewClock(epoch())).Take(t.Context(), "k", perMinute(), 1)
			if errs.CodeOf(err) != errs.CodeDBUnavailable || d != (ratelimit.Decision{}) {
				t.Fatalf("Take = %+v, %v, want db_unavailable", d, err)
			}
		})
	}
}

func TestDeleteRateLimitBucketsIdleBefore_deletesOnlyIdleBuckets(t *testing.T) {
	t.Parallel()
	pool, clk := testkit.DB(t), testkit.NewClock(epoch())
	l := newLimiter(t, pool, clk)
	take(t, l, "idle", perMinute(), 1)
	clk.Advance(time.Hour)
	take(t, l, "active", perMinute(), 1)
	n, err := sqlc.New(pool).DeleteRateLimitBucketsIdleBefore(t.Context(), epoch().Add(time.Minute))
	if err != nil || n != 1 {
		t.Fatalf("DeleteRateLimitBucketsIdleBefore = %d, %v, want 1", n, err)
	}
	var keys []string
	rows, err := pool.Query(t.Context(), `SELECT key FROM rate_limit_buckets`)
	if err == nil {
		keys, err = pgx.CollectRows(rows, pgx.RowTo[string])
	}
	if err != nil || len(keys) != 1 || keys[0] != "active" {
		t.Fatalf("buckets left = %v, %v, want [active]", keys, err)
	}
}
