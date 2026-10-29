package events

import (
	"time"

	"github.com/google/uuid"
)

const (
	TypeRankingSnapshotWritten  Type = "ranking.snapshot_written"
	TypeCallResolved            Type = "call.resolved"
	TypeRankingCallBoardWritten Type = "ranking.call_board_written"
	TypeRankingLedgerReady      Type = "ranking.ledger_ready"
)

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

type CallResolved struct {
	V              int         `json:"v"`
	ProposalID     uuid.UUID   `json:"proposal_id"`
	CabalID        uuid.UUID   `json:"cabal_id"`
	Kind           string      `json:"kind"`
	Symbol         string      `json:"symbol"`
	Outcome        string      `json:"outcome"`
	ResolvedAt     time.Time   `json:"resolved_at"`
	MoveBps        int         `json:"move_bps"`
	RightVoterIDs  []uuid.UUID `json:"right_voter_ids"  pii:"true"`
	MissedVoterIDs []uuid.UUID `json:"missed_voter_ids" pii:"true"`
	CabalRight     bool        `json:"cabal_right"`
}

func (CallResolved) Type() Type { return TypeCallResolved }

func (CallResolved) AggregateType() string { return rankingAggregate }

func (e CallResolved) AggregateID() uuid.UUID { return e.ProposalID }

type RankingCallBoardWritten struct {
	V          int       `json:"v"`
	RunID      uuid.UUID `json:"run_id"`
	ComputedAt time.Time `json:"computed_at"`
}

func (RankingCallBoardWritten) Type() Type { return TypeRankingCallBoardWritten }

func (RankingCallBoardWritten) AggregateType() string { return rankingAggregate }

func (e RankingCallBoardWritten) AggregateID() uuid.UUID { return e.RunID }

type RankingLedgerReady struct {
	V      int       `json:"v"`
	UserID uuid.UUID `json:"user_id" pii:"true"`
	Week   string    `json:"week"`
}

func (RankingLedgerReady) Type() Type { return TypeRankingLedgerReady }

func (RankingLedgerReady) AggregateType() string { return rankingAggregate }

func (e RankingLedgerReady) AggregateID() uuid.UUID { return e.UserID }
