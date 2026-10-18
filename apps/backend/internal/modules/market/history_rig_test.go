package market_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type historyRig struct {
	pool    *pgxpool.Pool
	clock   *testkit.Clock
	history *marketfake.PriceHistoryFake
	logs    *testkit.Logs
	day     time.Time
}

func newHistoryRig(t *testing.T) *historyRig {
	t.Helper()
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	return &historyRig{
		pool: testkit.DB(t), clock: clk, history: &marketfake.PriceHistoryFake{}, logs: &testkit.Logs{},
		day: clk.Now().Truncate(24 * time.Hour).Add(-72 * time.Hour),
	}
}

func (r *historyRig) ctx(t *testing.T) context.Context {
	t.Helper()
	return observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, r.logs))
}

func (r *historyRig) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := r.pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func (r *historyRig) points(t *testing.T, mint market.Mint) map[time.Time]string {
	t.Helper()
	rows, err := r.pool.Query(t.Context(),
		`SELECT ts, price_micros::text || '/' || source FROM price_points WHERE mint = $1`, mint.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[time.Time]string{}
	for rows.Next() {
		var ts time.Time
		var v string
		if err := rows.Scan(&ts, &v); err != nil {
			t.Fatal(err)
		}
		out[ts.UTC()] = v
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func sample(at time.Time, micros uint64) app.Sample { return app.Sample{At: at, Price: usd(micros)} }
