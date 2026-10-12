package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type SimulationAccountSource interface {
	Stored(ctx context.Context) (SimulationAccount, bool, error)
}

type SimulationCheck struct {
	accounts SimulationAccountSource
	code     AccountCode
	delegate chain.EVMAddress
}

var _ SimulationReadiness = SimulationCheck{}

func NewSimulationCheck(accounts SimulationAccountSource, code AccountCode, delegate chain.EVMAddress) SimulationCheck {
	return SimulationCheck{accounts: accounts, code: code, delegate: delegate}
}

func (c SimulationCheck) Ready(ctx context.Context, n chain.Network) (bool, error) {
	account, found, err := c.accounts.Stored(ctx)
	if err != nil || !found || !account.Covers(n) {
		return false, err
	}
	d, err := ReadDelegation(ctx, c.code, n, account.Address)
	return err == nil && d.Delegate == c.delegate, err
}
