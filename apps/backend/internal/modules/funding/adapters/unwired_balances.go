package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type UnwiredBalances struct{}

var _ port.Balances = UnwiredBalances{}

func (UnwiredBalances) Available(context.Context, ids.UserID) (port.Balance, error) {
	return port.Balance{}, errs.New(errs.CodeRPCUnavailable, "funding.UnwiredBalances.Available")
}
