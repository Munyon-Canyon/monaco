package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Unwired struct{}

var _ port.Queries = Unwired{}

func (Unwired) Positions(context.Context, ids.CabalID) ([]port.Position, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "treasury.Unwired.Positions")
}

func (Unwired) PotValue(context.Context, ids.CabalID) (money.Micros, error) {
	return money.Micros{}, errs.New(errs.CodePriceUnavailable, "treasury.Unwired.PotValue")
}

func (Unwired) TotalShares(context.Context, ids.CabalID) (money.SharesUnits, error) {
	return money.SharesUnits{}, errs.New(errs.CodeUpstreamUnavailable, "treasury.Unwired.TotalShares")
}

func (Unwired) ShareUnits(context.Context, ids.CabalID, ids.UserID) (money.SharesUnits, error) {
	return money.SharesUnits{}, errs.New(errs.CodeUpstreamUnavailable, "treasury.Unwired.ShareUnits")
}

func (Unwired) Stake(context.Context, ids.CabalID, ids.UserID) (port.Stake, error) {
	return port.Stake{}, errs.New(errs.CodeUpstreamUnavailable, "treasury.Unwired.Stake")
}

func (Unwired) StakesOf(context.Context, ids.UserID) ([]port.Stake, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "treasury.Unwired.StakesOf")
}

func (Unwired) ShareUnitsAt(context.Context, ids.CabalID, ids.UserID, time.Time) (money.SharesUnits, error) {
	return money.SharesUnits{}, errs.New(errs.CodeUpstreamUnavailable, "treasury.Unwired.ShareUnitsAt")
}

func (Unwired) CabalPositionsAt(context.Context, time.Time) ([]port.CabalPositions, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "treasury.Unwired.CabalPositionsAt")
}

func (Unwired) MemberStakesAt(context.Context, time.Time) ([]port.MemberStake, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "treasury.Unwired.MemberStakesAt")
}
