package chain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func TestStandInSignature_roundTripsTheTreasurysSignature(t *testing.T) {
	t.Parallel()
	stand := chain.StandInSignature("treasury-sig")
	if !chain.IsStandInSignature(stand) || chain.IsStandInSignature("treasury-sig") {
		t.Fatalf("IsStandInSignature(%q) must be true and a bare signature false", stand)
	}
	got, ok := chain.StandInTreasurySignature(stand)
	if !ok || got != "treasury-sig" {
		t.Fatalf("StandInTreasurySignature = %q, %v, want the treasury signature", got, ok)
	}
	if _, ok := chain.StandInTreasurySignature("treasury-sig"); ok {
		t.Fatal("a real signature has no treasury signature inside")
	}
	if _, ok := chain.StandInTreasurySignature(chain.StandInSignature("")); ok {
		t.Fatal("an empty stand-in has nothing to recover")
	}
}
