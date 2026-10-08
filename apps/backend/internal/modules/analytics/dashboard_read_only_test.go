package analytics_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/adapters"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestDashboard_StatementTimeout(t *testing.T) {
	t.Parallel()
	read := adapters.ReadOnly(testkit.DB(t), 50*time.Millisecond)
	err := read(t.Context(), func(ctx context.Context, db dbsqlc.DBTX) error {
		_, err := db.Exec(ctx, `SELECT pg_sleep(5)`)
		return err
	})
	if errs.CodeOf(err) != errs.CodeDashboardTimeout || errs.KindOf(errs.CodeDashboardTimeout) != errs.KindUnavailable {
		t.Fatalf("read error = %v, want a 503 dashboard_timeout", err)
	}
}

func TestDashboard_ReadOnly(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	err := adapters.ReadOnly(pool, time.Second)(t.Context(), func(ctx context.Context, db dbsqlc.DBTX) error {
		_, err := db.Exec(ctx, `INSERT INTO leaderboard_runs
			(run_id, as_of, prices_as_of, started_at, finished_at, rows_written, cabals_excluded)
			VALUES ($1, now(), now(), now(), now(), 0, 0)`, testkit.NewIDs(1).NewV7())
		return err
	})
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "25006" {
		t.Fatalf("write error = %v, want SQLSTATE 25006 read_only_sql_transaction", err)
	}
	var runs int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM leaderboard_runs`).Scan(&runs); err != nil || runs != 0 {
		t.Fatalf("leaderboard_runs = %d, %v, want the write refused", runs, err)
	}
}

func TestDashboard_ReadOnly_PassesOtherErrorsThrough(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test")
	err := adapters.ReadOnly(testkit.DB(t), time.Second)(t.Context(), func(context.Context, dbsqlc.DBTX) error {
		return boom
	})
	if !errors.Is(err, boom) || errs.CodeOf(err) == errs.CodeDashboardTimeout {
		t.Fatalf("error = %v, want boom and not a timeout", err)
	}
}
