package uniswapquote

import (
	"context"

	"golang.org/x/time/rate"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func (w *Wrapper) Cached() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.cache)
}

type StoredAccount struct {
	Address chain.EVMAddress
	Found   bool
	Err     error
	Reads   int
}

func (a *StoredAccount) SimulationAccount(context.Context) (chain.EVMAddress, bool, error) {
	a.Reads++
	return a.Address, a.Found, a.Err
}

func Stored(a chain.EVMAddress) *StoredAccount { return &StoredAccount{Address: a, Found: true} }

func (c *Client) WithLimit(l *rate.Limiter) *Client {
	c.limit = l
	return c
}
