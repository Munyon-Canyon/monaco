package replay

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type BackfillOptions struct {
	Pool    *pgxpool.Pool
	UoW     *db.UnitOfWork
	Clock   *Clock
	Now     clock.Clock
	Handler bus.HandlerSpec
	Types   []events.Type
	Since   uuid.UUID
}

func Backfill(ctx context.Context, o BackfillOptions) (Report, error) {
	for _, t := range o.Types {
		if t != o.Handler.Type() {
			return Report{}, errs.Wrap(RefusedError(fmt.Sprintf("handler %s handles %s, not %s",
				o.Handler.Name, o.Handler.Type(), t)), errs.CodeInvalidInput, "replay.Backfill")
		}
	}
	rows, err := load(ctx, o.Pool, `WHERE type = $1 AND id > $2`, string(o.Handler.Type()), o.Since)
	if err != nil {
		return Report{}, err
	}
	rep := Report{Events: len(rows)}
	for _, r := range rows {
		o.Clock.now = o.Now.Now()
		if err := deliver(ctx, o.UoW, o.Clock, o.Handler, r, &rep); err != nil {
			return rep, err
		}
	}
	return rep, nil
}
