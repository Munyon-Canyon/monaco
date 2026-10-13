package app

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func runOffHours(t *testing.T, sampleAge time.Duration) Valuation {
	t.Helper()
	now := valuationTime()
	f, usdc := heldAssetPorts(t, 1)
	equity := f.assetRows[0].ID
	f.asOfRows[equity] = market.Price{Micros: money.MicrosFromUint64(3_000_000), ObservedAt: now.Add(-sampleAge)}
	got, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f}, usdc,
	).Run(t.Context(), now)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if f.asOf != 1 || !f.asOfInstants[0].Equal(now) {
		t.Fatalf("PricesAsOf instants = %v, want one call at the run time", f.asOfInstants)
	}
	return got
}

func TestRunValuation_anEquityValuedOffHoursUsesTheNewestLiveSample(t *testing.T) {
	t.Parallel()
	got := runOffHours(t, time.Minute)
	if len(got.Cabals) != 1 || len(got.Flagged) != 0 || got.Cabals[0].Value != money.MicrosFromUint64(5_000_000) {
		t.Fatalf("Run() = %#v, want 1 USDC + 3 live + 1", got)
	}
}

func TestRunValuation_aSampleOlderThanFiveMinutesOffHoursExcludesTheCabal(t *testing.T) {
	t.Parallel()
	got := runOffHours(t, 5*time.Minute+time.Second)
	if len(got.Cabals) != 0 || len(got.Flagged) != 1 {
		t.Fatalf("Run() = %#v, want the cabal flagged and not valued", got)
	}
}
