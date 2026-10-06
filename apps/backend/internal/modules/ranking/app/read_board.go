package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
)

type Boards interface {
	LatestRun(context.Context) (domain.Run, bool, error)
	Page(ctx context.Context, board string, rng domain.Range, after, limit int) ([]domain.Entry, error)
	Row(ctx context.Context, board string, rng domain.Range, subject uuid.UUID) (domain.Entry, bool, error)
	Subjects(ctx context.Context, board string, rng domain.Range, subjects []uuid.UUID) ([]domain.Entry, error)
}

type PageKey struct {
	RunID  uuid.UUID
	Rev    int32
	Board  string
	Range  domain.Range
	Cursor int
	Limit  int
}

type PageLoader interface {
	Load(context.Context, PageKey, func(context.Context) (domain.BoardPage, error)) (domain.BoardPage, error)
}

type ReadBoard struct {
	Board   string
	Range   domain.Range
	Cursor  int
	Limit   int
	Viewer  *uuid.UUID
	Pages   PageLoader
	Friends Follows
}

func (r ReadBoard) Run(ctx context.Context, boards Boards) (domain.BoardPage, error) {
	run, ok, err := boards.LatestRun(ctx)
	if err != nil || !ok {
		return domain.BoardPage{}, err
	}
	if r.Friends != nil {
		return r.friendsPage(ctx, boards, run)
	}
	page, err := r.pageOf(ctx, boards, run)
	if err != nil {
		return domain.BoardPage{}, err
	}
	if r.Viewer != nil {
		if page.Me, err = r.MeFor(ctx, boards, *r.Viewer); err != nil {
			return domain.BoardPage{}, err
		}
	}
	return page, nil
}

func (r ReadBoard) pageOf(ctx context.Context, boards Boards, run domain.Run) (domain.BoardPage, error) {
	if r.Pages == nil {
		return r.PageAt(ctx, boards, run)
	}
	key := PageKey{RunID: run.ID, Rev: run.Rev, Board: r.Board, Range: r.Range, Cursor: r.Cursor, Limit: r.Limit}
	return r.Pages.Load(ctx, key, func(ctx context.Context) (domain.BoardPage, error) {
		return r.PageAt(ctx, boards, run)
	})
}

func (r ReadBoard) PageAt(ctx context.Context, boards Boards, run domain.Run) (domain.BoardPage, error) {
	rows, err := boards.Page(ctx, r.Board, r.Range, r.Cursor, r.Limit+1)
	if err != nil {
		return domain.BoardPage{}, err
	}
	page := domain.BoardPage{RunID: &run.ID, ComputedAt: run.FinishedAt, PricesAsOf: run.PricesAsOf, Rows: rows}
	if len(rows) > r.Limit {
		page.Rows = rows[:r.Limit]
		next := domain.EncodeCursor(page.Rows[r.Limit-1].Rank)
		page.NextCursor = &next
	}
	if len(page.Rows) > 0 {
		page.ComputedAt, page.PricesAsOf = page.Rows[0].ComputedAt, page.Rows[0].PricesAsOf
	}
	return page, nil
}

func (r ReadBoard) MeFor(ctx context.Context, boards Boards, viewer uuid.UUID) (*domain.Entry, error) {
	entry, ok, err := boards.Row(ctx, r.Board, r.Range, viewer)
	if err != nil || !ok {
		return nil, err
	}
	return &entry, nil
}
