package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	ReportsPageDefault = 50
	ReportsPageMax     = 100
)

type ReportsQuery struct {
	Status string
	After  *domain.Keyset
	Limit  int
}

type Report struct {
	ID        uuid.UUID
	Reporter  ids.UserID
	Handle    string
	Kind      string
	TargetID  uuid.UUID
	Reason    string
	Note      *string
	Status    string
	CreatedAt time.Time
}

type ReportsPage struct {
	Items []Report
	Next  *domain.Keyset
}

func ListReports(ctx context.Context, db sqlc.DBTX, users Users, q ReportsQuery) (ReportsPage, error) {
	const op = "social.ListReports"
	if q.Limit < 1 || q.Limit > ReportsPageMax || (q.Status != "open" && q.Status != "resolved") {
		return ReportsPage{}, errs.New(errs.CodeInvalidInput, op, slog.Int("limit", q.Limit),
			slog.String("status", q.Status))
	}
	params := sqlc.ListReportsParams{Status: q.Status, RowLimit: int32(q.Limit) + 1}
	if q.After != nil {
		params.HasCursor, params.AfterAt, params.AfterID = true, q.After.At, q.After.ID
	}
	rows, err := sqlc.New(db).ListReports(ctx, params)
	if err != nil {
		return ReportsPage{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	var next *domain.Keyset
	if len(rows) > q.Limit {
		rows = rows[:q.Limit]
		last := rows[q.Limit-1]
		next = &domain.Keyset{At: last.CreatedAt, ID: last.ID}
	}
	reporters := make([]ids.UserID, len(rows))
	for i, row := range rows {
		reporters[i] = ids.UserIDFrom(row.ReporterID)
	}
	cards := map[ids.UserID]UserCard{}
	if len(rows) > 0 {
		if cards, err = users.UsersByID(ctx, reporters); err != nil {
			return ReportsPage{}, errs.Wrap(err, errs.CodeOf(err), op)
		}
	}
	items := make([]Report, len(rows))
	for i, row := range rows {
		items[i] = reportOf(row, reporters[i], cards[reporters[i]])
	}
	return ReportsPage{Items: items, Next: next}, nil
}

func reportOf(row sqlc.Report, reporter ids.UserID, card UserCard) Report {
	report := Report{
		ID: row.ID, Reporter: reporter, Kind: row.Kind, TargetID: row.TargetID, Reason: row.Reason,
		Status: row.Status, CreatedAt: row.CreatedAt,
	}
	if !card.Deleted {
		report.Handle = card.Handle
	}
	if row.Note.Valid {
		report.Note = &row.Note.String
	}
	return report
}
