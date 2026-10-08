package adapters

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
)

func ReadOnly(pool *pgxpool.Pool, timeout time.Duration) app.ReadOnly {
	return func(ctx context.Context, fn func(ctx context.Context, q dbsqlc.DBTX) error) error {
		err := db.ReadOnly(ctx, pool, timeout, fn)
		if db.IsStatementTimeout(err) {
			return errs.Wrap(err, errs.CodeDashboardTimeout, "analytics.ReadOnly")
		}
		return err
	}
}
