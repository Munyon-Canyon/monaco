package app

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestCandidate_storableKeepsThePriceOnlyForTheLockedRowsMint(t *testing.T) {
	t.Parallel()
	price := money.MicrosFromUint64(709_800_000)
	c := candidate{mint: "mint-a", price: &price}
	if got, ok := c.storable("mint-a"); !ok || got != 709_800_000 {
		t.Fatalf("storable(same mint) = %d, %v, want 709800000", got, ok)
	}
	if got, ok := c.storable("mint-b"); ok {
		t.Fatalf("storable(other mint) = %d, %v, want none", got, ok)
	}
}
