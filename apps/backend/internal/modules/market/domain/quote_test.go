package domain

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestMicrosToInt64_rejectsAPricePastInt64(t *testing.T) {
	t.Parallel()
	got, ok := MicrosToInt64(money.MicrosFromUint64(uint64(math.MaxInt64) + 1))
	fit, fitOK := MicrosToInt64(money.MicrosFromUint64(uint64(math.MaxInt64)))
	if ok || got != 0 || !fitOK || fit != math.MaxInt64 {
		t.Fatalf("MicrosToInt64 overflow = %d %v, max = %d %v", got, ok, fit, fitOK)
	}
}
