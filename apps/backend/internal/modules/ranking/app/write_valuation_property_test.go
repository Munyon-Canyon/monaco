package app

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestSnapshotWriter_announcesExactlyTheRowsItInserts(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 50).Draw(t, "rows")
		entries := make([]Entry, n)
		for i := range entries {
			entries[i] = Entry{
				Board:     "people",
				Range:     "ALL",
				Rank:      i + 1,
				SubjectID: ids.Real{}.NewV7(),
				Flags:     []string{},
			}
		}
		queries := &snapshotQueriesFake{}
		appender := &eventAppenderFake{}
		valuation := Valuation{AsOf: valuationTime(), PricesAsOf: valuationTime(), Entries: entries}
		if err := (SnapshotWriter{}).persistQueries(
			t.Context(), queries, appender, uuid.Nil, valuation, valuationTime(), valuationTime(),
		); err != nil {
			t.Fatal(err)
		}
		var inserted []Entry
		if err := json.Unmarshal(queries.entries, &inserted); err != nil {
			t.Fatal(err)
		}
		announced, ok := appender.got.(events.RankingSnapshotWritten)
		if !ok || announced.RowsWritten != n || len(inserted) != n {
			t.Fatalf("announced %+v, inserted %d rows, want %d", appender.got, len(inserted), n)
		}
	})
}
