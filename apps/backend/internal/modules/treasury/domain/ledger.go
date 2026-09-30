package domain

import (
	"log/slog"
	"maps"
	"math"
	"math/big"
	"slices"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type CabalAccount string

const (
	CabalTreasury CabalAccount = "treasury"
	CabalVenue    CabalAccount = "venue"
	CabalMembers  CabalAccount = "members"
	CabalFees     CabalAccount = "fees"
)

type UserAccount string

const (
	UserWallet   UserAccount = "wallet"
	UserExternal UserAccount = "external"
	UserCabal    UserAccount = "cabal"
	UserHolder   UserAccount = "holder"
	UserIssuer   UserAccount = "issuer"
)

type CabalTxnKind string

const (
	CabalSwap    CabalTxnKind = "swap"
	CabalFund    CabalTxnKind = "fund"
	CabalCashOut CabalTxnKind = "cash_out"
)

type UserTxnKind string

const (
	UserDeposit    UserTxnKind = "deposit"
	UserWithdrawal UserTxnKind = "withdrawal"
	UserFund       UserTxnKind = "fund"
	UserCashOut    UserTxnKind = "cash_out"
)

type TxnStatus string

const (
	TxnPending TxnStatus = "pending"
	TxnSettled TxnStatus = "settled"
	TxnFailed  TxnStatus = "failed"
)

func transitions() map[TxnStatus][]TxnStatus {
	return map[TxnStatus][]TxnStatus{TxnPending: {TxnSettled, TxnFailed}}
}

func (s TxnStatus) CanMoveTo(to TxnStatus) bool { return slices.Contains(transitions()[s], to) }

type Asset string

func MintAsset(mint chain.SolanaAddress) Asset { return Asset(mint) }

func SharesAsset(cabal ids.CabalID) Asset { return Asset("shares:" + cabal.String()) }

type Entry[A CabalAccount | UserAccount] struct {
	Account A
	Asset   Asset
	Amount  money.SignedMicros
}

type (
	CabalEntry = Entry[CabalAccount]
	UserEntry  = Entry[UserAccount]
)

type CabalTxnHeader struct {
	ID          uuid.UUID
	CabalID     ids.CabalID
	Kind        CabalTxnKind
	Status      TxnStatus
	SwapID      uuid.UUID
	TransferID  uuid.UUID
	TxSignature chain.Signature
}

type CabalTxn struct {
	CabalTxnHeader
	entries []CabalEntry
}

func NewCabalTxn(h CabalTxnHeader, entries []CabalEntry) (CabalTxn, error) {
	if err := balanced("treasury.NewCabalTxn", entries); err != nil {
		return CabalTxn{}, err
	}
	return CabalTxn{CabalTxnHeader: h, entries: slices.Clone(entries)}, nil
}

func (t CabalTxn) Entries() []CabalEntry { return slices.Clone(t.entries) }

type UserTxnHeader struct {
	ID          uuid.UUID
	UserID      ids.UserID
	CabalID     ids.CabalID
	Kind        UserTxnKind
	Status      TxnStatus
	TransferID  uuid.UUID
	TxSignature chain.Signature
}

type UserTxn struct {
	UserTxnHeader
	entries []UserEntry
}

func NewUserTxn(h UserTxnHeader, entries []UserEntry) (UserTxn, error) {
	if err := balanced("treasury.NewUserTxn", entries); err != nil {
		return UserTxn{}, err
	}
	return UserTxn{UserTxnHeader: h, entries: slices.Clone(entries)}, nil
}

func (t UserTxn) Entries() []UserEntry { return slices.Clone(t.entries) }

func balanced[A CabalAccount | UserAccount](op string, entries []Entry[A]) error {
	if len(entries) == 0 || len(entries) > math.MaxInt16 {
		return errs.New(errs.CodeInvalidInput, op, slog.Int("entries", len(entries)))
	}
	sums := map[Asset]*big.Int{}
	for i, e := range entries {
		if e.Amount.IsZero() || e.Asset == "" {
			return errs.New(errs.CodeInvalidInput, op, slog.Int("entry", i), slog.String("asset", string(e.Asset)))
		}
		if sums[e.Asset] == nil {
			sums[e.Asset] = new(big.Int)
		}
		sums[e.Asset].Add(sums[e.Asset], big.NewInt(e.Amount.Int64()))
	}
	for _, asset := range slices.Sorted(maps.Keys(sums)) {
		if sums[asset].Sign() != 0 {
			return errs.New(errs.CodeLedgerUnbalanced, op,
				slog.String("asset", string(asset)), slog.String("sum", sums[asset].String()))
		}
	}
	return nil
}
