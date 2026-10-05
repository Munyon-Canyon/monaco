package domain

import (
	"math"
	"math/big"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const fullBps = 10000

func WeightsBps(values []money.Micros) ([]int32, error) {
	raw := make([]uint64, len(values))
	for i, v := range values {
		raw[i] = v.Uint64()
	}
	return splitBps(raw, "treasury.WeightsBps")
}

func SlicesBps(units []money.SharesUnits) ([]int32, error) {
	raw := make([]uint64, len(units))
	for i, u := range units {
		raw[i] = u.Uint64()
	}
	return splitBps(raw, "treasury.SlicesBps")
}

func splitBps(values []uint64, op string) ([]int32, error) {
	out := make([]int32, len(values))
	var total money.Micros
	largest := 0
	for i, v := range values {
		var err error
		total, err = total.Add(money.MicrosFromUint64(v))
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeOf(err), op)
		}
		if v > values[largest] {
			largest = i
		}
	}
	if total.IsZero() {
		return out, nil
	}
	remainder := int32(fullBps)
	for i, v := range values {
		out[i] = floorBps(v, total.Uint64())
		remainder -= out[i]
	}
	out[largest] += remainder
	return out, nil
}

func floorBps(part, total uint64) int32 {
	q := new(big.Int).Mul(new(big.Int).SetUint64(part), big.NewInt(fullBps))
	return saturateInt32(q.Quo(q, new(big.Int).SetUint64(total)))
}

func ReturnBps(pnl, net money.SignedMicros) (int32, bool) {
	if net.Int64() <= 0 {
		return 0, false
	}
	q := new(big.Int).Mul(big.NewInt(pnl.Int64()), big.NewInt(fullBps))
	return saturateInt32(q.Quo(q, big.NewInt(net.Int64()))), true
}

func saturateInt32(q *big.Int) int32 {
	if !q.IsInt64() {
		q = big.NewInt(int64(q.Sign()) * math.MaxInt64)
	}
	v := q.Int64()
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}
