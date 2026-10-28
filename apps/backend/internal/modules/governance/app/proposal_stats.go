package app

import (
	"math"
	"math/big"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const bpsPerWhole = 10_000

type Stats struct {
	TreasuryShareBps *int64
	ClosesAt         time.Time
	ClosedAt         *time.Time
}

type MyVote struct {
	Order, Of, YesOrder, NoOrder int
	Decided                      bool
}

func TreasuryShareBps(value, pot money.Micros) (*int64, error) {
	share, ok, err := shareBps(value, pot)
	if err != nil || !ok {
		return nil, err
	}
	return &share, nil
}

func shareBps(value, pot money.Micros) (int64, bool, error) {
	if pot.IsZero() {
		return 0, false, nil
	}
	share, err := money.MulDiv(value.Uint64(), bpsPerWhole, pot.Uint64())
	if err != nil || share > math.MaxInt64 {
		return 0, false, err
	}
	return new(big.Int).SetUint64(share).Int64(), true, nil
}

func rowShareValue(kind string, usdc, quoteOut int64) money.Micros {
	if kind == string(domain.KindSell) {
		return money.MicrosFromUint64(unsigned(quoteOut))
	}
	return money.MicrosFromUint64(unsigned(usdc))
}

func rowStats(r sqlc.GetProposalRow) (Stats, error) {
	out := Stats{ClosesAt: r.ExpiresAt}
	if r.ClosedAt.Valid {
		closed := r.ClosedAt.Time
		out.ClosedAt = &closed
	}
	if !r.PotValueMicros.Valid {
		return out, nil
	}
	value := rowShareValue(r.Kind, r.UsdcMicros.Int64, r.QuoteOutAmount)
	share, err := TreasuryShareBps(value, money.MicrosFromUint64(unsigned(r.PotValueMicros.Int64)))
	out.TreasuryShareBps = share
	return out, err
}

func myVoteOf(row sqlc.BallotOrderRow, decided bool) *MyVote {
	return &MyVote{
		Order: int(row.Order), Of: int(row.Voters), YesOrder: int(row.YesOrder), NoOrder: int(row.NoOrder),
		Decided: decided,
	}
}
