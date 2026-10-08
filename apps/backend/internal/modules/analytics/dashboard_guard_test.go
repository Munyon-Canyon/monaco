package analytics_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bucket"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/analyticsapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type probe struct {
	db  dbsqlc.DBTX
	run func(ctx context.Context, db dbsqlc.DBTX) error
}

func (p probe) LedgerTotals(ctx context.Context, _, _ time.Time, _ bucket.Size) ([]treasuryport.LedgerBucket, error) {
	return nil, p.run(ctx, p.db)
}

func (probe) PlatformBalanceTotal(context.Context) (money.Micros, error) { return money.Micros{}, nil }

func (probe) LatestRun(context.Context) (rankingport.Run, error) { return rankingport.Run{}, nil }

func (probe) LatestCabalValues(context.Context) ([]rankingport.CabalValue, error) { return nil, nil }

func probeMoney(pool *pgxpool.Pool, timeout time.Duration, run func(context.Context, dbsqlc.DBTX) error) app.Money {
	return app.Money{
		Read: adapters.ReadOnly(pool, timeout),
		Bind: func(db dbsqlc.DBTX) app.MoneySources {
			p := probe{db: db, run: run}
			return app.MoneySources{Ledger: p, Valuations: p}
		},
	}
}

func sleepFor(d string) func(context.Context, dbsqlc.DBTX) error {
	return func(ctx context.Context, db dbsqlc.DBTX) error {
		_, err := db.Exec(ctx, "SELECT pg_sleep("+d+")")
		return err
	}
}

func TestDashboard_Money_StatementTimeoutAnswers503(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	reads := probeMoney(pool, 50*time.Millisecond, sleepFor("5"))
	h := dashboardHandler(t, pool, func(m api.Mount) { analyticsapi.Mount(adapters.HTTP{Money: reads}, m) })
	w := moneyGet(t, h, viewerToken, window("2026-09-01", "2026-09-02", "day"))
	if w.Code != http.StatusServiceUnavailable || problemCode(t, w) != string(errs.CodeDashboardTimeout) {
		t.Fatalf("status = %d %s, want 503 dashboard_timeout", w.Code, w.Body)
	}
}
