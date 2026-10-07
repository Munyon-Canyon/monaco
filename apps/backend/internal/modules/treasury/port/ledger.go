package port

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type TxnScope string

const (
	TxnScopeUser  TxnScope = "user"
	TxnScopeCabal TxnScope = "cabal"
)

type TxnCursor struct {
	At time.Time
	ID uuid.UUID
}

type TxnHeader struct {
	ID          uuid.UUID
	Scope       TxnScope
	UserID      *ids.UserID
	CabalID     *ids.CabalID
	Kind        string
	Status      string
	SwapID      *ids.SwapID
	TransferID  *uuid.UUID
	TxSignature chain.Signature
	CreatedAt   time.Time
}

type TxnEntry struct {
	Seq     int16
	Account string
	Asset   string
	Amount  int64
}

type Txn struct {
	TxnHeader
	Entries []TxnEntry
}

type Share struct {
	CabalID    ids.CabalID
	UserID     ids.UserID
	ShareUnits money.SharesUnits
}

type TxnReader interface {
	UserTxns(ctx context.Context, user ids.UserID, cursor *TxnCursor, limit int) ([]TxnHeader, error)
	CabalTxns(ctx context.Context, cabal ids.CabalID, cursor *TxnCursor, limit int) ([]TxnHeader, error)
	TxnByID(ctx context.Context, id uuid.UUID) (Txn, bool, error)
	TxnBySignature(ctx context.Context, sig chain.Signature) (Txn, bool, error)
}

type ShareReader interface {
	UserShares(ctx context.Context, user ids.UserID) ([]Share, error)
	CabalShares(ctx context.Context, cabal ids.CabalID) ([]Share, error)
}

type RawHolding struct {
	Mint  chain.SolanaAddress
	Units money.BaseUnits
}

type HoldingReader interface {
	CabalHoldings(ctx context.Context, cabal ids.CabalID) ([]RawHolding, error)
}

type LedgerReader interface {
	TxnReader
	ShareReader
	HoldingReader
}
