package port

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Withdrawals interface {
	OpenWithdrawals(ctx context.Context, user ids.UserID, page WithdrawalPage) ([]OpenWithdrawal, error)
}

type WithdrawalPage struct {
	Before *WithdrawalCursor
	Limit  int32
}

type WithdrawalCursor struct {
	At time.Time
	ID uuid.UUID
}

type OpenWithdrawal struct {
	ID          uuid.UUID
	Failed      bool
	Delta       money.SignedMicros
	TxSignature *string
	CreatedAt   time.Time
}
