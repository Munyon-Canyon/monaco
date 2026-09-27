package testkit

import (
	"crypto/rand"
	"encoding/binary"
	"flag"
	"testing"
)

var seedFlag = flag.Uint64("testkit.seed", 0, "replay a failure with the seed testkit.RandSeed logged")

func RandSeed(tb testing.TB) uint64 {
	tb.Helper()
	seed := *seedFlag
	if seed == 0 {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			tb.Fatalf("testkit.RandSeed: %v", err)
		}
		seed = binary.BigEndian.Uint64(b[:])
	}
	tb.Cleanup(func() {
		if tb.Failed() {
			tb.Logf("testkit: rerun with -testkit.seed=%d", seed)
		}
	})
	return seed
}
