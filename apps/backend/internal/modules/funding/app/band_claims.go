package app

import (
	"sync"

	"github.com/google/uuid"

	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type bandClaims struct {
	mu sync.Mutex
	by map[uuid.UUID]chain.Signature
}

func (c *bandClaims) claim(bands []fundingport.ArrivalBand, sig chain.Signature, got money.Micros) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	live := make(map[uuid.UUID]bool, len(bands))
	for _, b := range bands {
		live[b.ID] = true
	}
	for id := range c.by {
		if !live[id] {
			delete(c.by, id)
		}
	}
	for _, b := range bands {
		if c.by[b.ID] == sig && b.Contains(got) {
			return true
		}
	}
	for _, b := range bands {
		if _, taken := c.by[b.ID]; !taken && b.Contains(got) {
			c.by[b.ID] = sig
			return true
		}
	}
	return false
}
