package domain

import (
	"math"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func MicrosToInt64(m money.Micros) (int64, bool) {
	v := m.Uint64()
	if v > uint64(math.MaxInt64) {
		return 0, false
	}
	return int64(v), true
}
