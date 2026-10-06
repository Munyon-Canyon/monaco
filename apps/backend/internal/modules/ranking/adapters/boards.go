package adapters

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Boards struct{ DB sqlc.DBTX }

var _ app.Boards = Boards{}

func (b Boards) LatestRun(ctx context.Context) (domain.Run, bool, error) {
	run, err := sqlc.New(b.DB).LastLeaderboardRun(ctx)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return domain.Run{}, false, nil
	case err != nil:
		return domain.Run{}, false, errs.Wrap(err, errs.CodeInternal, "ranking.Boards.LatestRun")
	}
	return domain.Run{ID: run.RunID, Rev: run.Rev, PricesAsOf: run.PricesAsOf, FinishedAt: run.FinishedAt}, true, nil
}

func (b Boards) Page(
	ctx context.Context, board string, rng domain.Range, after, limit int,
) ([]domain.Entry, error) {
	const op = "ranking.Boards.Page"
	if after < 0 || after > math.MaxInt32 || limit < 1 || limit > math.MaxInt32 {
		return nil, errs.New(errs.CodeInvalidInput, op, slog.Int("after", after), slog.Int("limit", limit))
	}
	rows, err := sqlc.New(b.DB).BoardPage(ctx, sqlc.BoardPageParams{
		Board: board, Range: string(rng), AfterRank: int32(after), RowLimit: int32(limit),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	entries := make([]domain.Entry, 0, len(rows))
	for _, row := range rows {
		entry, err := entryFrom(row)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (b Boards) Row(
	ctx context.Context, board string, rng domain.Range, subject uuid.UUID,
) (domain.Entry, bool, error) {
	row, err := sqlc.New(b.DB).BoardRowFor(ctx, sqlc.BoardRowForParams{
		Board: board, Range: string(rng), SubjectID: subject,
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return domain.Entry{}, false, nil
	case err != nil:
		return domain.Entry{}, false, errs.Wrap(err, errs.CodeInternal, "ranking.Boards.Row")
	}
	entry, err := entryFrom(row)
	return entry, err == nil, err
}

func entryFrom(row sqlc.LeaderboardEntry) (domain.Entry, error) {
	value, err := money.SignedMicrosFromInt64(row.ValueMicros).Micros()
	if err != nil {
		return domain.Entry{}, errs.Wrap(err, errs.CodeInternal, "ranking.entryFrom")
	}
	flags := make([]domain.Flag, 0, len(row.Flags))
	for _, f := range row.Flags {
		flags = append(flags, domain.Flag(f))
	}
	entry := domain.Entry{
		Rank: int(row.Rank),
		Subject: domain.Subject{
			ID: row.SubjectID, Name: row.SubjectName,
			Handle: textPtr(row.SubjectHandle), PictureURL: textPtr(row.SubjectPictureUrl),
		},
		Value:      value,
		PnL:        money.SignedMicrosFromInt64(row.PnlMicros),
		Flags:      flags,
		ComputedAt: row.ComputedAt,
		PricesAsOf: row.PricesAsOf,
	}
	if row.ReturnBps.Valid {
		bps := domain.Bps(row.ReturnBps.Int64)
		entry.Return = &bps
	}
	return entry, nil
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}
