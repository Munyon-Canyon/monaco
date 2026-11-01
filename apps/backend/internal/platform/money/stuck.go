package money

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const (
	StuckWithdrawal       = "withdrawal"
	StuckFundTransfer     = "fund_transfer"
	StuckCashOutJob       = "cash_out_job"
	StuckCashOutPayout    = "cash_out_payout"
	StuckWindDown         = "wind_down"
	StuckEVMForward       = "evm_forward"
	StuckExternalDeposit  = "external_deposit"
	StuckOnrampSession    = "onramp_session"
	StuckDepositCandidate = "deposit_candidate"
)

type StuckRow struct {
	Kind    string
	Ref     string
	UserID  uuid.UUID
	CabalID uuid.UUID
	Status  string
	Since   time.Time
}

type StuckSummary struct {
	Kind   string
	Count  int
	Oldest time.Time
}

func StuckKinds() []string {
	return []string{
		StuckWithdrawal, StuckFundTransfer, StuckCashOutJob, StuckCashOutPayout, StuckWindDown,
		StuckEVMForward, StuckExternalDeposit, StuckOnrampSession, StuckDepositCandidate,
	}
}

func IsIntentKind(kind string) bool {
	return kind == StuckWithdrawal || kind == StuckFundTransfer || kind == StuckCashOutJob
}

type StuckReader interface {
	List(ctx context.Context, kind string, olderThan time.Duration, limit int) ([]StuckRow, error)
	Summary(ctx context.Context, olderThan time.Duration) ([]StuckSummary, error)
}
