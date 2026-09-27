package db

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
)

func GuardedUpdate(ctx context.Context, q sqlc.DBTX, sql string, args ...any) (bool, error) {
	const op = "db.GuardedUpdate"
	tag, err := q.Exec(ctx, sql, args...)
	if err != nil {
		return false, classify(err, op)
	}
	switch n := tag.RowsAffected(); n {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, errs.New(errs.CodeInternal, op, slog.Int64("rows", n))
	}
}
