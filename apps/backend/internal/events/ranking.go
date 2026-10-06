package events

import (
	"time"

	"github.com/google/uuid"
)

const TypeRankingSnapshotWritten Type = "ranking.snapshot_written"

const rankingAggregate = "ranking"

type RankingSnapshotWritten struct {
	V              int       `json:"v"`
	RunID          uuid.UUID `json:"run_id"`
	AsOf           time.Time `json:"as_of"`
	PricesAsOf     time.Time `json:"prices_as_of"`
	ComputedAt     time.Time `json:"computed_at"`
	RowsWritten    int       `json:"rows_written"`
	CabalsExcluded int       `json:"cabals_excluded"`
}

func (RankingSnapshotWritten) Type() Type { return TypeRankingSnapshotWritten }

func (RankingSnapshotWritten) AggregateType() string { return rankingAggregate }

func (e RankingSnapshotWritten) AggregateID() uuid.UUID { return e.RunID }
