package app

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	socialport "github.com/monaco/monaco/apps/backend/internal/modules/social/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type (
	AssetID    = market.AssetID
	AssetPrice = market.Price
)

func AssetIDFrom(u uuid.UUID) AssetID { return market.AssetIDFrom(u) }

type Prices interface {
	PricesAsOf(context.Context, []market.AssetID, time.Time) (map[market.AssetID]market.Price, error)
}

type Market interface {
	Prices
	AssetsByMints(context.Context, []string) ([]market.Asset, error)
	ListingPrices(context.Context, []market.ListingKey, time.Time) (map[market.ListingKey]market.ListingPrice, error)
}

type Treasury interface {
	ValuationInputsAt(context.Context, time.Time) (treasury.ValuationInputs, error)
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

type Ballots = governanceport.Ballots

type Follows = interface {
	FollowingIDs(context.Context, ids.UserID) ([]ids.UserID, error)
}

type UnwiredFollows struct{}

var _ Follows = UnwiredFollows{}

func (UnwiredFollows) FollowingIDs(context.Context, ids.UserID) ([]ids.UserID, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "ranking.UnwiredFollows.FollowingIDs")
}

type RangeSnapshots interface {
	SnapshotsAt(context.Context, []time.Time) ([]sqlc.SnapshotsAtRow, error)
}

type Ports struct {
	Ballots   Ballots
	Market    Market
	Treasury  Treasury
	Funding   Funding
	Cabals    Cabals
	Users     Users
	Follows   Follows
	Previous  PreviousReader
	Snapshots RangeSnapshots
}

type Ownership interface {
	OwnershipBps(context.Context, ids.UserID) (map[ids.CabalID]int32, error)
}

type MemberRoles interface {
	MembersOf(context.Context, []ids.CabalID) (map[ids.CabalID][]cabal.MemberView, error)
}

type (
	DisplayBalances = fundingport.DisplayBalances
	Balance         = fundingport.Balance
)

type Blocks interface {
	BlockedByMe(ctx context.Context, viewer, user ids.UserID) (bool, error)
}

type FollowCounts = interface {
	Counts(context.Context, ids.UserID) (int, int, error)
	FollowedByMe(context.Context, ids.UserID, ids.UserID) (bool, error)
	FollowedAmong(context.Context, ids.UserID, []ids.UserID) (map[ids.UserID]bool, error)
	BlockedByMe(context.Context, ids.UserID, ids.UserID) (bool, error)
}

type UnwiredBlocks struct{}

var _ Blocks = UnwiredBlocks{}

func (UnwiredBlocks) BlockedByMe(context.Context, ids.UserID, ids.UserID) (bool, error) {
	return false, errs.New(errs.CodeUpstreamUnavailable, "ranking.UnwiredBlocks.BlockedByMe")
}

type Trades interface {
	FilledBuysBetween(ctx context.Context, cabals []ids.CabalID, start, end time.Time) ([]trading.FilledBuy, error)
}

type Replies interface {
	TopReply(
		ctx context.Context, proposalIDs []ids.ProposalID, start, end time.Time,
	) (*socialport.TopReply, error)
}
