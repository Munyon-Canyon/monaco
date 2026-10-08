package port

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/bucket"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Flow struct {
	Count int64
	USDC  money.Micros
}

type LedgerBucket struct {
	Start       time.Time
	Deposits    Flow
	Funds       Flow
	CashOuts    Flow
	Withdrawals Flow
	SwapBuy     money.Micros
	SwapSell    money.Micros
}

type Dashboard interface {
	LedgerTotals(ctx context.Context, from, to time.Time, size bucket.Size) ([]LedgerBucket, error)
	PlatformBalanceTotal(ctx context.Context) (money.Micros, error)
}
