package replay

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func verifiedTables(ctx context.Context, o Options) ([]string, error) {
	if len(o.Tables) > 0 {
		return o.Tables, nil
	}
	skip := []string{"events", "event_deliveries", "idempotency_keys"}
	for _, c := range o.Checks {
		skip = append(skip, c.Tables...)
	}
	tables, _ := o.Target.Query(ctx, `SELECT tablename::text FROM pg_tables
		WHERE schemaname = 'public' AND NOT tablename = ANY($1) ORDER BY 1`, skip)
	names, err := pgx.CollectRows(tables, pgx.RowTo[string])
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "replay.verify")
	}
	return names, nil
}

func verify(ctx context.Context, o Options) ([]string, error) {
	const op = "replay.verify"
	names, err := verifiedTables(ctx, o)
	if err != nil {
		return nil, err
	}
	var diffs []string
	for _, table := range names {
		var sides [2][]string
		for i, pool := range []*pgxpool.Pool{o.Source, o.Target} {
			if sides[i], err = sortedRows(ctx, pool, table); err != nil {
				return nil, err
			}
		}
		diffs = append(diffs, diffRows(table, sides[0], sides[1])...)
	}
	for _, c := range o.Checks {
		found, err := c.Check(ctx, o.Source)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeOf(err), op)
		}
		for _, d := range found {
			diffs = append(diffs, "ledger "+c.Name+": "+d)
		}
	}
	return diffs, nil
}
