package adapters

import (
	"math"
	"testing"
)

func TestClamp32_holdsTheWireRange(t *testing.T) {
	t.Parallel()
	for in, want := range map[int]int32{0: 0, 7: 7, math.MaxInt32: math.MaxInt32, math.MaxInt32 + 1: math.MaxInt32, -1: math.MaxInt32} {
		if got := clamp32(in); got != want {
			t.Fatalf("clamp32(%d) = %d, want %d", in, got, want)
		}
	}
}
