package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type UnwiredHoldings struct{}

var (
	_ Balances = UnwiredHoldings{}
	_ Stakes   = UnwiredHoldings{}
)

func (UnwiredHoldings) Observe(context.Context, ids.UserID) (fundingport.Observation, error) {
	return fundingport.Observation{}, errs.New(errs.CodeUpstreamUnavailable, "identity.UnwiredHoldings.Observe")
}

func (UnwiredHoldings) AvailableIn(
	context.Context, dbsqlc.DBTX, fundingport.Observation,
) (fundingport.Balance, error) {
	return fundingport.Balance{}, errs.New(errs.CodeUpstreamUnavailable, "identity.UnwiredHoldings.AvailableIn")
}

func (UnwiredHoldings) StakesOf(context.Context, ids.UserID) ([]treasuryport.Stake, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "identity.UnwiredHoldings.StakesOf")
}

type UnwiredDeleteChecks struct{}

var (
	_ Arriving = UnwiredDeleteChecks{}
	_ Leaders  = UnwiredDeleteChecks{}
)

func (UnwiredDeleteChecks) DepositArriving(context.Context, dbsqlc.DBTX, ids.UserID, time.Time) (bool, error) {
	return false, errs.New(errs.CodeUpstreamUnavailable, "identity.UnwiredDeleteChecks.DepositArriving")
}

func (UnwiredDeleteChecks) LeadsCabalWithMembers(context.Context, dbsqlc.DBTX, ids.UserID) (bool, error) {
	return false, errs.New(errs.CodeUpstreamUnavailable, "identity.UnwiredDeleteChecks.LeadsCabalWithMembers")
}
