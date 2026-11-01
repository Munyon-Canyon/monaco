package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const BlocksPageMax = 500

type BlockedUser struct {
	ID   ids.UserID
	Card UserCard
}

type BlocksQuery struct {
	Viewer ids.UserID
	After  *domain.Keyset
	Limit  int
}

type BlocksPage struct {
	Items []BlockedUser
	Next  *domain.Keyset
}

func ListBlocks(ctx context.Context, db sqlc.DBTX, users Users, q BlocksQuery) (BlocksPage, error) {
	const op = "social.ListBlocks"
	if q.Limit < 1 || q.Limit > BlocksPageMax {
		return BlocksPage{}, errs.New(errs.CodeInvalidInput, op, slog.Int("limit", q.Limit))
	}
	params := sqlc.ListUserBlocksParams{BlockerID: q.Viewer.UUID(), RowLimit: int32(q.Limit + 1)}
	if q.After != nil {
		params.HasCursor, params.AfterAt, params.AfterID = true, q.After.At, q.After.ID
	}
	rows, err := sqlc.New(db).ListUserBlocks(ctx, params)
	if err != nil {
		return BlocksPage{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	page := BlocksPage{Items: []BlockedUser{}}
	if len(rows) > q.Limit {
		rows = rows[:q.Limit]
		last := rows[len(rows)-1]
		page.Next = &domain.Keyset{At: last.CreatedAt.UTC(), ID: last.ID}
	}
	if len(rows) == 0 {
		return page, nil
	}
	blocked := make([]ids.UserID, len(rows))
	for i, row := range rows {
		blocked[i] = ids.UserIDFrom(row.BlockedID)
	}
	cards, err := users.UsersByID(ctx, blocked)
	if err != nil {
		return BlocksPage{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	page.Items = make([]BlockedUser, len(blocked))
	for i, id := range blocked {
		page.Items[i] = BlockedUser{ID: id, Card: cards[id]}
	}
	return page, nil
}
