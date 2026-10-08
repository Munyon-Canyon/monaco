package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type PendingBounce struct {
	CabalID ids.CabalID
	Since   time.Time
}

func BounceQueue(ctx context.Context, q sqlc.DBTX, limit int) ([]PendingBounce, error) {
	rows, err := sqlc.New(q).ListBouncePauses(ctx, int64(limit))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, "funding.BounceQueue")
	}
	out := make([]PendingBounce, len(rows))
	for i, r := range rows {
		out[i] = PendingBounce{CabalID: ids.CabalIDFrom(r.CabalID), Since: r.CreatedAt}
	}
	return out, nil
}
