package domain

import (
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ActivityKind string

const (
	ActivityBuy     ActivityKind = "buy"
	ActivitySell    ActivityKind = "sell"
	ActivityFund    ActivityKind = "fund"
	ActivityCashOut ActivityKind = "cash_out"
)

type ActivityStatus string

const (
	ActivityPending   ActivityStatus = "pending"
	ActivityConfirmed ActivityStatus = "confirmed"
	ActivityFailed    ActivityStatus = "failed"
)

type Activity struct {
	ID          uuid.UUID
	CabalID     ids.CabalID
	Actor       ids.UserID
	Kind        ActivityKind
	Status      ActivityStatus
	Asset       Asset
	USDCMicros  *uint64
	Units       *uint64
	TxSignature chain.Signature
}

type Trade struct {
	SwapID      uuid.UUID
	CabalID     ids.CabalID
	Action      string
	In          Leg
	Out         Leg
	TxSignature chain.Signature
}

func TradeActivity(t Trade, status ActivityStatus) (Activity, error) {
	a := Activity{ID: t.SwapID, CabalID: t.CabalID, Status: status, TxSignature: t.TxSignature}
	cash, asset := t.In, t.Out
	switch t.Action {
	case string(ActivityBuy):
		a.Kind = ActivityBuy
	case string(ActivitySell):
		a.Kind = ActivitySell
		cash, asset = t.Out, t.In
	default:
		return Activity{}, errs.New(errs.CodeInvalidInput, "treasury.TradeActivity", slog.String("action", t.Action))
	}
	a.Asset = asset.Asset
	a.USDCMicros, a.Units = optional(cash.Amount), optional(asset.Amount)
	return a, nil
}

func optional(v uint64) *uint64 {
	if v == 0 {
		return nil
	}
	return &v
}

type Fund struct {
	TransferID  uuid.UUID
	CabalID     ids.CabalID
	UserID      ids.UserID
	USDCMicros  uint64
	ShareUnits  uint64
	TxSignature chain.Signature
}

func FundActivity(f Fund, status ActivityStatus) Activity {
	return Activity{
		ID: f.TransferID, CabalID: f.CabalID, Actor: f.UserID, Kind: ActivityFund, Status: status,
		USDCMicros: optional(f.USDCMicros), Units: optional(f.ShareUnits), TxSignature: f.TxSignature,
	}
}
