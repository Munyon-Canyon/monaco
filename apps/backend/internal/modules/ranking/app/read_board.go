package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
)

type ViewBoards interface {
	ViewPage(
		ctx context.Context, board string, rng domain.Range, view domain.View, after, limit int,
	) ([]domain.Entry, error)
	ViewRow(
		ctx context.Context, board string, rng domain.Range, view domain.View, subject uuid.UUID,
	) (domain.Entry, bool, error)
}

type Boards interface {
	ViewBoards
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
	View   domain.View
	Cursor int
	Limit  int
}

type PageLoader interface {
	Load(context.Context, PageKey, func(context.Context) (domain.BoardPage, error)) (domain.BoardPage, error)
}

type ReadBoard struct {
	Board   string
	Range   domain.Range
	View    domain.View
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
	key := PageKey{
		RunID: run.ID, Rev: run.Rev, Board: r.Board, Range: r.Range, View: r.View, Cursor: r.Cursor, Limit: r.Limit,
	}
	return r.Pages.Load(ctx, key, func(ctx context.Context) (domain.BoardPage, error) {
		return r.PageAt(ctx, boards, run)
	})
}

func (r ReadBoard) PageAt(ctx context.Context, boards Boards, run domain.Run) (domain.BoardPage, error) {
	rows, err := r.pageRows(ctx, boards)
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

func (r ReadBoard) pageRows(ctx context.Context, boards Boards) ([]domain.Entry, error) {
	if r.View.IsDefault() {
		return boards.Page(ctx, r.Board, r.Range, r.Cursor, r.Limit+1)
	}
	return boards.ViewPage(ctx, r.Board, r.Range, r.View, r.Cursor, r.Limit+1)
}

func (r ReadBoard) MeFor(ctx context.Context, boards Boards, viewer uuid.UUID) (*domain.Entry, error) {
	var entry domain.Entry
	var ok bool
	var err error
	if r.View.Sort == "" || r.View.Sort == domain.SortReturn {
		entry, ok, err = boards.Row(ctx, r.Board, r.Range, viewer)
	} else {
		entry, ok, err = boards.ViewRow(ctx, r.Board, r.Range, r.View, viewer)
	}
	if err != nil || !ok {
		return nil, err
	}
	return &entry, nil
}
