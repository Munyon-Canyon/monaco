package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type UserReader interface {
	UsersByID(ctx context.Context, userIDs []ids.UserID) (map[ids.UserID]identityport.UserCard, error)
	UserByHandle(ctx context.Context, handle string) (identityport.UserCard, error)
}

type WalletReader interface {
	MemberWallet(ctx context.Context, id ids.UserID) (identityport.MemberWallet, error)
}

type CabalReader interface {
	Cabal(ctx context.Context, id ids.CabalID) (cabalport.CabalView, error)
}

type CabalIndex interface {
	Cabals(ctx context.Context, cabalIDs []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error)
	CabalsOf(ctx context.Context, user ids.UserID) ([]ids.CabalID, error)
}

type CabalDetails interface {
	Members(ctx context.Context, id ids.CabalID) ([]cabalport.MemberView, error)
	Rules(ctx context.Context, id ids.CabalID) (cabalport.Rules, error)
	TreasuryWallet(ctx context.Context, id ids.CabalID) (cabalport.TreasuryWallet, error)
}

type TxnLists interface {
	UserTxns(ctx context.Context, user ids.UserID, cursor *treasuryport.TxnCursor, limit int) (
		[]treasuryport.TxnHeader, error)
	CabalTxns(ctx context.Context, cabal ids.CabalID, cursor *treasuryport.TxnCursor, limit int) (
		[]treasuryport.TxnHeader, error)
}

type ShareLists interface {
	UserShares(ctx context.Context, user ids.UserID) ([]treasuryport.Share, error)
	CabalShares(ctx context.Context, cabal ids.CabalID) ([]treasuryport.Share, error)
}

type Holdings interface {
	CabalHoldings(ctx context.Context, cabal ids.CabalID) ([]treasuryport.RawHolding, error)
}

type Assets interface {
	ListAll(ctx context.Context) ([]market.Asset, error)
}

type EventHistory interface {
	EventsByAggregate(ctx context.Context, aggregateType string, id uuid.UUID, types []string, limit int) (
		[]bus.EventRow, error)
}

type ActionLog interface {
	Recent(ctx context.Context, targetType, targetID string, limit int) ([]sqlc.AdminAction, error)
}
