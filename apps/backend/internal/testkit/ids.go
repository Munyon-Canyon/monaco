package testkit

import (
	"encoding/binary"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type IDs struct {
	mu   sync.Mutex
	rng  *rand.Rand
	next time.Time
}

var _ ids.Generator = (*IDs)(nil)

func NewIDs(seed uint64) *IDs {
	return &IDs{
		rng:  rand.New(rand.NewPCG(seed, 0)),
		next: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func (g *IDs) NewV7() uuid.UUID {
	g.mu.Lock()
	defer g.mu.Unlock()
	var u uuid.UUID
	binary.BigEndian.PutUint64(u[0:8], uint64(g.next.UnixMilli())<<16|g.rng.Uint64()&0xffff)
	binary.BigEndian.PutUint64(u[8:16], g.rng.Uint64())
	u[6] = u[6]&0x0f | 0x70
	u[8] = u[8]&0x3f | 0x80
	g.next = g.next.Add(time.Millisecond)
	return u
}
