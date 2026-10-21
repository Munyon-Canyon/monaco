package domain

import (
	"cmp"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	HighRiskWithdrawalMicros = 1_000_000_000
	StepUpTTL                = 5 * time.Minute
	StepUpFreshness          = 2 * time.Minute
)

func (r WithdrawalRequest) RequiresStepUp(knownDestination bool) bool {
	return !knownDestination || r.Amount.Cmp(money.MicrosFromUint64(HighRiskWithdrawalMicros)) >= 0
}

func (r WithdrawalRequest) Chain() chain.Network { return cmp.Or(r.Network, chain.NetworkSolana) }
