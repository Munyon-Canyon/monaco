package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const UnreadCap = 100

type Unread struct{ db sqlc.DBTX }

func NewUnread(db sqlc.DBTX) Unread { return Unread{db: db} }

func (u Unread) UnreadCounts(
	ctx context.Context, user ids.UserID, cabals []ids.CabalID,
) (map[ids.CabalID]int, error) {
	const op = "social.UnreadCounts"
	raw := make([]uuid.UUID, len(cabals))
	for i, c := range cabals {
		raw[i] = c.UUID()
	}
	rows, err := sqlc.New(u.db).UnreadCounts(ctx, sqlc.UnreadCountsParams{UserID: user.UUID(), CabalIds: raw})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	counts := make(map[ids.CabalID]int, len(rows))
	for _, row := range rows {
		counts[ids.CabalIDFrom(row.CabalID)] = int(row.Unread)
	}
	return counts, nil
}
