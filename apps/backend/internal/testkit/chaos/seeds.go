package chaos

import (
	"flag"
	"os"
	"strconv"
	"testing"
)

const defaultSeeds = 50

var seedFlag = flag.Uint64("chaos.seed", 0, "replay only this chaos seed")

func Seeds(tb testing.TB) []Seed {
	tb.Helper()
	if *seedFlag != 0 {
		return []Seed{Seed(*seedFlag)}
	}
	return seedRange(tb, os.Getenv("CHAOS_SEEDS"))
}

func seedRange(tb testing.TB, raw string) []Seed {
	tb.Helper()
	n := uint64(defaultSeeds)
	if raw != "" {
		var err error
		if n, err = strconv.ParseUint(raw, 10, 32); err != nil || n == 0 {
			tb.Fatalf("chaos: CHAOS_SEEDS=%q, want a positive integer", raw)
		}
	}
	out := make([]Seed, n)
	for i := range out {
		out[i] = Seed(i + 1)
	}
	return out
}
