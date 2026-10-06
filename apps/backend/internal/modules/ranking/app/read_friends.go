package app

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (r ReadBoard) friendsPage(ctx context.Context, boards Boards, run domain.Run) (domain.BoardPage, error) {
	const op = "ranking.ReadBoard.friendsPage"
	if r.Viewer == nil {
		return domain.BoardPage{}, errs.New(errs.CodeInternal, op)
	}
	viewer := *r.Viewer
	following, err := r.Friends.FollowingIDs(ctx, ids.UserIDFrom(viewer))
	if err != nil {
		return domain.BoardPage{}, err
	}
	subjects := make([]uuid.UUID, 0, len(following)+1)
	for _, id := range following {
		subjects = append(subjects, id.UUID())
	}
	if !slices.Contains(subjects, viewer) {
		subjects = append(subjects, viewer)
	}
	rows, err := boards.Subjects(ctx, r.Board, r.Range, subjects)
	if err != nil {
		return domain.BoardPage{}, err
	}
	var me *domain.Entry
	for i := range rows {
		rows[i].Rank = i + 1
		if rows[i].Subject.ID == viewer {
			me = &rows[i]
		}
	}
	page := domain.BoardPage{RunID: &run.ID, ComputedAt: run.FinishedAt, PricesAsOf: run.PricesAsOf, Me: me}
	if r.Cursor < len(rows) {
		page.Rows = rows[r.Cursor:min(r.Cursor+r.Limit, len(rows))]
	}
	if r.Cursor+r.Limit < len(rows) {
		next := domain.EncodeCursor(r.Cursor + r.Limit)
		page.NextCursor = &next
	}
	if len(page.Rows) > 0 {
		page.ComputedAt, page.PricesAsOf = page.Rows[0].ComputedAt, page.Rows[0].PricesAsOf
	}
	return page, nil
}
