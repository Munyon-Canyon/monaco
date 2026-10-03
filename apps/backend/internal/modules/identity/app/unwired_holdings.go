package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type UnwiredHoldings struct{}

var (
	_ Balances = UnwiredHoldings{}
	_ Stakes   = UnwiredHoldings{}
)

func (UnwiredHoldings) Available(context.Context, ids.UserID) (fundingport.Balance, error) {
	return fundingport.Balance{}, errs.New(errs.CodeUpstreamUnavailable, "identity.UnwiredHoldings.Available")
}

func (UnwiredHoldings) StakesOf(context.Context, ids.UserID) ([]treasuryport.Stake, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "identity.UnwiredHoldings.StakesOf")
}
