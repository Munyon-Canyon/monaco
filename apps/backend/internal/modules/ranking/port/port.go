package port

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Run struct {
	RunID          uuid.UUID
	AsOf           time.Time
	PricesAsOf     time.Time
	StartedAt      time.Time
	FinishedAt     time.Time
	RowsWritten    int
	CabalsExcluded int
	Rev            int
}

type CabalValue struct {
	CabalID     ids.CabalID
	At          time.Time
	Value       money.Micros
	NavPerShare money.Micros
	TotalShares money.SharesUnits
}

type Range = domain.Range

const Range1M = domain.Range1M

type PeriodKind = domain.PeriodKind

const (
	PeriodMonth      = domain.PeriodMonth
	MinResolvedCalls = domain.MinResolvedCalls
)

type PersonReturn struct {
	ReturnBps *int64
	PnlMicros money.SignedMicros
	AsOf      time.Time
}

type CabalReturn struct {
	ReturnBps *int64
	AsOf      time.Time
}

type CabalReturnsReader interface {
	CabalReturns(ctx context.Context, rng Range, cabals []ids.CabalID) (map[ids.CabalID]CabalReturn, error)
}

type Queries interface {
	LatestRun(ctx context.Context) (Run, error)
	LatestCabalValues(ctx context.Context) ([]CabalValue, error)
	PeopleReturns(ctx context.Context, rng Range, users []ids.UserID) (map[ids.UserID]PersonReturn, error)
}

type Period = domain.Period

type CallRecord struct {
	Right          int
	Resolved       int
	Voted          int
	Eligible       int
	PendingSymbols []string
}

type Calls interface {
	Records(
		ctx context.Context,
		userIDs []ids.UserID,
		cabalID *ids.CabalID,
		p Period,
	) (map[ids.UserID]CallRecord, error)
	CabalRecords(ctx context.Context, cabalIDs []ids.CabalID, p Period) (map[ids.CabalID]CallRecord, error)
}

type Standing struct {
	Rank        *int
	Of          int
	RateBps     *int
	Right       int
	Resolved    int
	PeriodStart time.Time
}

type Leader struct {
	SubjectID   uuid.UUID
	Name        string
	Handle      string
	PictureURL  string
	RateBps     int
	Right       int
	Resolved    int
	PeriodStart time.Time
}

type CallBoards interface {
	CallStanding(ctx context.Context, userID ids.UserID, period domain.PeriodKind) (Standing, error)
	CallLeader(ctx context.Context, board domain.Board, period domain.PeriodKind) (Leader, error)
}
