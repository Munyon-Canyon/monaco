package poller

import (
	"crypto/rand"
	"encoding/binary"
	"math"
	"time"
)

func Spread(period time.Duration) time.Duration {
	if period <= 0 {
		return 0
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	return time.Duration(min(binary.BigEndian.Uint64(b[:])%uint64(period), math.MaxInt64))
}
