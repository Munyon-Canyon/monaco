package market_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func usd(micros uint64) money.Micros { return money.MicrosFromUint64(micros) }

func newestFirst(at time.Time, micros ...uint64) []domain.Sample {
	out := make([]domain.Sample, len(micros))
	for i, m := range micros {
		out[i] = domain.Sample{Micros: usd(m), ObservedAt: at.Add(-time.Duration(i) * 2 * time.Minute)}
	}
	return out
}

func wantAccepted(t *testing.T, samples []domain.Sample, want domain.Sample) {
	t.Helper()
	got, ok := domain.Accept(samples)
	if !ok || got != want {
		t.Fatalf("Accept(%v) = %v, %v, want %v", samples, got, ok, want)
	}
}

func TestAccept_JumpHeldOneSample(t *testing.T) {
	t.Parallel()
	samples := newestFirst(clock.Real{}.Now(), 125_000_000, 100_000_000, 100_000_000)
	wantAccepted(t, samples, samples[1])
}

func TestAccept_JumpConfirmed(t *testing.T) {
	t.Parallel()
	samples := newestFirst(clock.Real{}.Now(), 125_000_000, 125_000_000, 100_000_000)
	wantAccepted(t, samples, samples[0])
}

func TestAccept_aMoveOfExactlyTwentyPercentStands(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now()
	for _, micros := range []uint64{120_000_000, 80_000_000, 100_000_000} {
		samples := newestFirst(now, micros, 100_000_000)
		wantAccepted(t, samples, samples[0])
	}
	for _, micros := range []uint64{120_000_001, 79_999_999} {
		samples := newestFirst(now, micros, 100_000_000)
		wantAccepted(t, samples, samples[1])
	}
}

func TestAccept_twoJumpsInARowFallBackToTheOldestOfThree(t *testing.T) {
	t.Parallel()
	samples := newestFirst(clock.Real{}.Now(), 45_000_000, 70_000_000, 100_000_000)
	wantAccepted(t, samples, samples[2])
}

func TestAccept_readsOnlyTheNewestThreeSamples(t *testing.T) {
	t.Parallel()
	samples := newestFirst(clock.Real{}.Now(), 300_000_000, 200_000_000, 130_000_000, 100_000_000)
	wantAccepted(t, samples, samples[2])
}

func TestAccept_oneSampleStandsAndNoSampleIsUnpriced(t *testing.T) {
	t.Parallel()
	samples := newestFirst(clock.Real{}.Now(), 0)
	wantAccepted(t, samples, samples[0])
	if got, ok := domain.Accept(nil); ok || got != (domain.Sample{}) {
		t.Fatalf("Accept(nil) = %v, %v, want no price", got, ok)
	}
}

func TestAccept_aJumpFromZeroIsHeld(t *testing.T) {
	t.Parallel()
	samples := newestFirst(clock.Real{}.Now(), 1, 0)
	wantAccepted(t, samples, samples[1])
}
