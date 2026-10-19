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

type PersonReturn struct {
	ReturnBps *int64
	PnlMicros money.SignedMicros
	AsOf      time.Time
}

type Queries interface {
	LatestRun(ctx context.Context) (Run, error)
	LatestCabalValues(ctx context.Context) ([]CabalValue, error)
	PeopleReturns(ctx context.Context, rng Range, users []ids.UserID) (map[ids.UserID]PersonReturn, error)
}
