package domain_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestRequiresStepUp_aNewDestinationAlwaysDoesAndAKnownOneDoesFromTheHighRiskAmountUp(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		amount := rapid.OneOf(
			rapid.Uint64Range(domain.MinWithdrawalMicros, 5_000_000_000),
			rapid.Uint64Range(domain.HighRiskWithdrawalMicros-2, domain.HighRiskWithdrawalMicros+2),
		).Draw(t, "amount")
		request := domain.WithdrawalRequest{Amount: money.MicrosFromUint64(amount), To: wallet}
		if !request.RequiresStepUp(false) {
			t.Fatalf("a new destination for %d micros needs no step-up", amount)
		}
		if got, want := request.RequiresStepUp(true), amount >= domain.HighRiskWithdrawalMicros; got != want {
			t.Fatalf("a known destination for %d micros: RequiresStepUp = %v, want %v", amount, got, want)
		}
	})
}

func TestWithdrawalRequest_chainDefaultsToSolana(t *testing.T) {
	t.Parallel()
	if got := (domain.WithdrawalRequest{}).Chain(); got != chain.NetworkSolana {
		t.Fatalf("Chain() = %q, want solana", got)
	}
	if got := (domain.WithdrawalRequest{Network: chain.NetworkBase}).Chain(); got != chain.NetworkBase {
		t.Fatalf("Chain() = %q, want base", got)
	}
}
