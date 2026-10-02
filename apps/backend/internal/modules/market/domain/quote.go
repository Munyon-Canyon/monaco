package domain

import (
	"math"
	"math/big"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const SparklinePoints = 48

func QuoteBps(current, reference money.Micros) (int32, bool) {
	ref := reference.Uint64()
	if ref == 0 {
		return 0, false
	}
	delta := new(big.Int).Sub(new(big.Int).SetUint64(current.Uint64()), new(big.Int).SetUint64(ref))
	bps := delta.Mul(delta, big.NewInt(10_000)).Quo(delta, new(big.Int).SetUint64(ref))
	if !bps.IsInt64() {
		return 0, false
	}
	v := bps.Int64()
	if v < math.MinInt32 || v > math.MaxInt32 {
		return 0, false
	}
	return int32(v), true
}

func MicrosToInt64(m money.Micros) (int64, bool) {
	v := m.Uint64()
	if v > uint64(math.MaxInt64) {
		return 0, false
	}
	return int64(v), true
}

func DisplayUntil(now time.Time, newestFirst []Sample) (time.Time, Sample, bool) {
	accepted, ok := Accept(newestFirst)
	if !ok {
		return time.Time{}, Sample{}, false
	}
	if newestFirst[0] != accepted {
		return accepted.ObservedAt, accepted, true
	}
	return now, accepted, true
}

func LastSparkline(closes, newestFirst []Sample) []money.Micros {
	accepted, ok := Accept(newestFirst)
	if !ok {
		return nil
	}
	kept := holdLastClose(dropAfter(closes, accepted.ObservedAt), newestFirst, accepted)
	if len(kept) > SparklinePoints {
		kept = kept[len(kept)-SparklinePoints:]
	}
	return microsOf(kept)
}

func dropAfter(closes []Sample, at time.Time) []Sample {
	kept := make([]Sample, 0, len(closes))
	for _, close := range closes {
		if close.ObservedAt.After(at) {
			continue
		}
		kept = append(kept, close)
	}
	return kept
}

func holdLastClose(closes, newestFirst []Sample, accepted Sample) []Sample {
	if len(closes) == 0 {
		return closes
	}
	last := closes[len(closes)-1].Micros
	if !rejectedPrice(last, newestFirst, accepted) && withinAFifth(last, accepted.Micros) {
		return closes
	}
	out := slices.Clone(closes)
	out[len(out)-1].Micros = accepted.Micros
	return out
}

func rejectedPrice(price money.Micros, newestFirst []Sample, accepted Sample) bool {
	for _, sample := range newestFirst {
		if sample != accepted && sample.Micros == price {
			return true
		}
	}
	return false
}

func microsOf(closes []Sample) []money.Micros {
	out := make([]money.Micros, len(closes))
	for i := range closes {
		out[i] = closes[i].Micros
	}
	return out
}
