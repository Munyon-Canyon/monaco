package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
)

type closedRow struct {
	Board       string    `json:"board"`
	Period      string    `json:"period"`
	PeriodStart string    `json:"period_start"`
	Rank        *int      `json:"rank"`
	SubjectID   uuid.UUID `json:"subject_id"`
	RateBps     *int      `json:"rate_bps"`
	RightCount  int       `json:"right_count"`
	Resolved    int       `json:"resolved"`
	Voted       int       `json:"voted"`
	Eligible    int       `json:"eligible"`
}

func closePeriods(ctx context.Context, q *sqlc.Queries, closing []rankedInput, closedAt time.Time) error {
	for _, in := range closing {
		if len(in.entries) == 0 {
			continue
		}
		rows := make([]closedRow, len(in.entries))
		for i, e := range in.entries {
			rows[i] = closedRow{
				Board: string(
					in.board,
				),
				Period:      string(in.period.Kind),
				PeriodStart: in.period.Start.Format(time.DateOnly),
				Rank:        e.Rank,
				SubjectID:   e.ID,
				RateBps:     e.RateBps,
				RightCount:  e.Right,
				Resolved:    e.Resolved,
				Voted:       e.Voted,
				Eligible:    e.Eligible,
			}
		}
		encoded, _ := json.Marshal(rows)
		if err := q.InsertClosedBoard(
			ctx,
			sqlc.InsertClosedBoardParams{Rows: encoded, ClosedAt: closedAt},
		); err != nil {
			return err
		}
	}
	return nil
}
