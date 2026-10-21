package fakes

import (
	"context"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type Balances struct {
	testkit.Faults
	mu       sync.Mutex
	balances map[ids.UserID]funding.Balance
}

var _ funding.Balances = (*Balances)(nil)

func NewBalances() *Balances { return &Balances{balances: map[ids.UserID]funding.Balance{}} }

func (f *Balances) Set(user ids.UserID, balance funding.Balance) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.balances[user] = balance
}

func (f *Balances) Available(_ context.Context, user ids.UserID) (funding.Balance, error) {
	if err := f.Check("Available"); err != nil {
		return funding.Balance{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.balances[user], nil
}

func (f *Balances) Observe(_ context.Context, user ids.UserID) (funding.Observation, error) {
	if err := f.Check("Available"); err != nil {
		return funding.Observation{}, err
	}
	return funding.Observation{User: user}, nil
}

func (f *Balances) AvailableIn(_ context.Context, _ dbsqlc.DBTX, obs funding.Observation) (funding.Balance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.balances[obs.User], nil
}
