package port

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Balances interface {
	Available(ctx context.Context, user ids.UserID) (Balance, error)
}

type Balance struct {
	OnChainMicros            money.Micros
	InFlightFundMicros       money.Micros
	InFlightWithdrawalMicros money.Micros
	AvailableMicros          money.Micros
	AsOf                     time.Time
}
