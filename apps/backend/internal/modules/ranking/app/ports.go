package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Prices interface {
	PricesAsOf(context.Context, []market.AssetID, time.Time) (map[market.AssetID]market.Price, error)
}

type Calendar interface {
	Session(context.Context, market.AssetID, time.Time) (market.SessionInfo, error)
}

type Market interface {
	Prices
	Calendar
}

type Treasury interface {
	CabalPositionsAt(context.Context, time.Time) ([]treasury.CabalPositions, error)
	MemberStakesAt(context.Context, time.Time) ([]treasury.MemberStake, error)
	MemberFlowsBetween(context.Context, time.Time, time.Time) ([]treasury.MemberFlow, error)
}

type Funding interface {
	PausedCabals(context.Context) (funding.PausedSet, error)
}

type Cabals interface {
	AllCabals(context.Context) ([]cabal.View, error)
	MembersOf(context.Context, []ids.CabalID) (map[ids.CabalID][]cabal.MemberView, error)
}

type Users interface {
	UsersByID(context.Context, []ids.UserID) (map[ids.UserID]identity.UserCard, error)
}

type Ports struct {
	Market   Market
	Treasury Treasury
	Funding  Funding
	Cabals   Cabals
	Users    Users
}
