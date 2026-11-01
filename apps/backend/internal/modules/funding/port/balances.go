package port

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Balances interface {
	Available(ctx context.Context, user ids.UserID) (Balance, error)
	Observe(ctx context.Context, user ids.UserID) (Observation, error)
	AvailableIn(ctx context.Context, q dbsqlc.DBTX, obs Observation) (Balance, error)
}

type Observation struct {
	User              ids.UserID
	OnChainMicros     money.Micros
	SpendableMicros   money.Micros
	UnspendableMicros money.Micros
	Slot              uint64
	At                time.Time
	Signatures        map[chain.Signature]bool
}

type DisplayBalances interface {
	ForDisplay(ctx context.Context, user ids.UserID) (Balance, error)
}

type SignatureOwner interface {
	OwnsSignature(ctx context.Context, sig chain.Signature) (bool, error)
}

type UserSignatureOwner interface {
	OwnsSignatureForUser(ctx context.Context, sig chain.Signature, user ids.UserID) (bool, error)
}

type Balance struct {
	OnChainMicros            money.Micros
	InFlightFundMicros       money.Micros
	InFlightWithdrawalMicros money.Micros
	AvailableMicros          money.Micros
	UnspendableMicros        money.Micros
	AsOf                     time.Time
}

type DepositsArriving interface {
	DepositArriving(ctx context.Context, q dbsqlc.DBTX, user ids.UserID, at time.Time) (bool, error)
}
