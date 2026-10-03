package domain

import (
	"math"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestChangeBps_measuresTheMoveInBasisPoints(t *testing.T) {
	t.Parallel()
	up, ok := QuoteBps(money.MicrosFromUint64(110_000_000), money.MicrosFromUint64(100_000_000))
	down, downOK := QuoteBps(money.MicrosFromUint64(80_000_000), money.MicrosFromUint64(100_000_000))
	flat, flatOK := QuoteBps(money.MicrosFromUint64(100_000_000), money.MicrosFromUint64(100_000_000))
	_, zeroRef := QuoteBps(money.MicrosFromUint64(100_000_000), money.MicrosFromUint64(0))
	if !ok || !downOK || !flatOK || zeroRef || up != 1000 || down != -2000 || flat != 0 {
		t.Fatalf("ChangeBps = %d %v, %d %v, %d %v, zeroRef %v", up, ok, down, downOK, flat, flatOK, zeroRef)
	}
}

func TestChangeBps_refusesAMoveThatDoesNotFit(t *testing.T) {
	t.Parallel()
	_, tooWide := QuoteBps(money.MicrosFromUint64(math.MaxUint64), money.MicrosFromUint64(1))
	_, pastInt32 := QuoteBps(money.MicrosFromUint64(300_000), money.MicrosFromUint64(1))
	if tooWide || pastInt32 {
		t.Fatal("ChangeBps accepted a move past int32")
	}
}

func TestMicrosToInt64_rejectsAPricePastInt64(t *testing.T) {
	t.Parallel()
	got, ok := MicrosToInt64(money.MicrosFromUint64(uint64(math.MaxInt64) + 1))
	fit, fitOK := MicrosToInt64(money.MicrosFromUint64(uint64(math.MaxInt64)))
	if ok || got != 0 || !fitOK || fit != math.MaxInt64 {
		t.Fatalf("MicrosToInt64 overflow = %d %v, max = %d %v", got, ok, fit, fitOK)
	}
}

func TestDisplayUntil_holdsASpikeAndKeepsAConfirmedPrice(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	spike := []Sample{
		{Micros: money.MicrosFromUint64(200), ObservedAt: now},
		{Micros: money.MicrosFromUint64(100), ObservedAt: now.Add(-time.Minute)},
		{Micros: money.MicrosFromUint64(100), ObservedAt: now.Add(-2 * time.Minute)},
	}
	until, accepted, ok := DisplayUntil(now, spike)
	if !ok || accepted != spike[1] || !until.Equal(spike[1].ObservedAt) {
		t.Fatalf("DisplayUntil(spike) = %v, %+v, %v", until, accepted, ok)
	}
	confirmed := []Sample{{Micros: money.MicrosFromUint64(100), ObservedAt: now}}
	until, accepted, ok = DisplayUntil(now, confirmed)
	if !ok || accepted != confirmed[0] || !until.Equal(now) {
		t.Fatalf("DisplayUntil(confirmed) = %v, %+v, %v", until, accepted, ok)
	}
	if _, _, ok := DisplayUntil(now, nil); ok {
		t.Fatal("DisplayUntil(nil) priced an empty book")
	}
}

func TestLastSparkline_holdsTheSpikeAndKeepsTheNewest48(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	newest := []Sample{
		{Micros: money.MicrosFromUint64(200), ObservedAt: now},
		{Micros: money.MicrosFromUint64(110), ObservedAt: now.Add(-time.Minute)},
		{Micros: money.MicrosFromUint64(100), ObservedAt: now.Add(-2 * time.Minute)},
	}
	held := LastSparkline([]Sample{
		{Micros: money.MicrosFromUint64(100)},
		{Micros: money.MicrosFromUint64(200)},
	}, newest)
	if len(held) != 2 || held[1] != money.MicrosFromUint64(110) {
		t.Fatalf("LastSparkline(spike) = %v", held)
	}
	confirmed := []Sample{{Micros: money.MicrosFromUint64(110), ObservedAt: now}}
	kept := LastSparkline([]Sample{{Micros: money.MicrosFromUint64(110)}}, confirmed)
	if len(kept) != 1 || kept[0] != confirmed[0].Micros {
		t.Fatalf("LastSparkline(confirmed) = %v", kept)
	}
	closes := make([]Sample, SparklinePoints+1)
	for i := range closes {
		closes[i] = Sample{Micros: money.MicrosFromUint64(uint64(i + 1))}
	}
	trimmed := LastSparkline(closes, confirmed)
	if len(trimmed) != SparklinePoints || trimmed[0] != money.MicrosFromUint64(2) {
		t.Fatalf("LastSparkline(long) len = %d, first = %v", len(trimmed), trimmed[0])
	}
	if LastSparkline(nil, nil) != nil {
		t.Fatal("LastSparkline(unpriced) was not nil")
	}
	if got := LastSparkline(nil, newest); len(got) != 0 {
		t.Fatalf("LastSparkline(priced, empty) = %v", got)
	}
	skipped := LastSparkline([]Sample{{Micros: money.MicrosFromUint64(90)}}, newest)
	if len(skipped) != 1 || skipped[0] != money.MicrosFromUint64(90) {
		t.Fatalf("LastSparkline(other close) = %v", skipped)
	}
}

func TestLastSparkline_dropsHeldClosesThatCrossABucket(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	at := func(hour, minute int) time.Time {
		return day.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
	}
	newest := []Sample{
		{Micros: money.MicrosFromUint64(300), ObservedAt: at(15, 0)},
		{Micros: money.MicrosFromUint64(200), ObservedAt: at(14, 58)},
		{Micros: money.MicrosFromUint64(100), ObservedAt: at(14, 56)},
	}
	got := LastSparkline([]Sample{
		{Micros: money.MicrosFromUint64(300), ObservedAt: at(13, 30)},
		{Micros: money.MicrosFromUint64(200), ObservedAt: at(14, 30)},
		{Micros: money.MicrosFromUint64(300), ObservedAt: at(15, 0)},
	}, newest)
	if len(got) != 2 || got[0] != money.MicrosFromUint64(300) || got[1] != money.MicrosFromUint64(100) {
		t.Fatalf("LastSparkline(cross bucket) = %v", got)
	}
	if later := LastSparkline(
		[]Sample{{Micros: money.MicrosFromUint64(300), ObservedAt: at(15, 0)}},
		newest,
	); len(
		later,
	) != 0 {
		t.Fatalf("LastSparkline(only a later bucket) = %v", later)
	}
}

func TestLastSparkline_holdsASpikeThatLandsAfterTheQuote(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	at := func(hour, minute int) time.Time {
		return day.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
	}
	newest := []Sample{{Micros: money.MicrosFromUint64(100), ObservedAt: at(14, 56)}}
	got := LastSparkline([]Sample{
		{Micros: money.MicrosFromUint64(80), ObservedAt: at(13, 30)},
		{Micros: money.MicrosFromUint64(400), ObservedAt: at(14, 30)},
		{Micros: money.MicrosFromUint64(500), ObservedAt: at(15, 0)},
	}, newest)
	if len(got) != 2 || got[0] != money.MicrosFromUint64(80) || got[1] != money.MicrosFromUint64(100) {
		t.Fatalf("LastSparkline(raced spike) = %v", got)
	}
	small := LastSparkline([]Sample{
		{Micros: money.MicrosFromUint64(100), ObservedAt: at(14, 30)},
		{Micros: money.MicrosFromUint64(110), ObservedAt: at(14, 30)},
	}, newest)
	if len(small) != 2 || small[1] != money.MicrosFromUint64(110) {
		t.Fatalf("LastSparkline(small move) = %v", small)
	}
}
