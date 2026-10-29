package ranking_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	callBoardPeople = 10_000
	callBoardP95    = 50 * time.Millisecond
)

func BenchmarkCallBoard_screen(b *testing.B) {
	pool := testkit.DB(b)
	at := clock.Real{}.Now().UTC()
	run := ids.Real{}.NewV7()
	subjects := make([]uuid.UUID, callBoardPeople)
	rows := make([]map[string]any, callBoardPeople)
	for i := range rows {
		subjects[i] = ids.Real{}.NewV7()
		right := 10_000 - i
		rows[i] = map[string]any{
			"board": "people", "period": "month", "period_start": at.Format(time.DateOnly), "subject_id": subjects[i],
			"subject_kind": "person", "subject_name": "Person", "subject_handle": "person",
			"subject_created_at": at.Format(time.RFC3339Nano), "rank": i + 1, "right_count": right, "resolved": 10_000,
			"voted": 10_000, "eligible": 10_000, "rate_bps": right, "turnout_bps": 10_000, "movement": 0,
			"movement_kind": "same", "run_id": run, "computed_at": at.Format(time.RFC3339Nano),
		}
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		b.Fatal(err)
	}
	q := sqlc.New(pool)
	if err := q.InsertCallBoardEntries(b.Context(), raw); err != nil {
		b.Fatal(err)
	}
	if err := q.InsertCallBoardRun(
		b.Context(), sqlc.InsertCallBoardRunParams{RunID: run, StartedAt: at, FinishedAt: at},
	); err != nil {
		b.Fatal(err)
	}
	if _, err := pool.Exec(b.Context(), "ANALYZE call_board_entries"); err != nil {
		b.Fatal(err)
	}
	clk := clock.Real{}
	read := app.ReadCallBoard{Runs: q, Ranks: q, Subjects: q, Heads: adapters.NewCallBoardHeadCache(8), Clock: clk}
	query := app.CallBoardQuery{
		Viewer: ids.UserIDFrom(subjects[callBoardPeople/2]), Board: domain.BoardPeople, Period: domain.PeriodMonth,
	}
	took := make([]time.Duration, 0, b.N)
	for b.Loop() {
		start := clk.Now()
		view, err := read.Run(b.Context(), query)
		if err != nil || view.Me == nil || *view.Me.Rank != callBoardPeople/2+1 || len(view.Around) != 7 {
			b.Fatalf("Run = %+v, %v, want the viewer at rank %d with 7 rows around", view.Me, err, callBoardPeople/2+1)
		}
		took = append(took, clk.Now().Sub(start))
	}
	slices.Sort(took)
	p95 := took[len(took)*95/100]
	b.Logf("p95 = %s over %d runs", p95, len(took))
	if p95 > callBoardP95 {
		b.Fatalf("p95 = %s, want under %s", p95, callBoardP95)
	}
}
