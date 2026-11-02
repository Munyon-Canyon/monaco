package domain

import (
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
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
	Units       *money.Units
	TxSignature chain.Signature
	CashOutSale bool
}

type Trade struct {
	SwapID      uuid.UUID
	CabalID     ids.CabalID
	Action      string
	In          Leg
	Out         Leg
	Fee         uint64
	TxSignature chain.Signature
}

func TradeActivity(t Trade, status ActivityStatus) (Activity, error) {
	a := Activity{ID: t.SwapID, CabalID: t.CabalID, Status: status, TxSignature: t.TxSignature}
	cash, asset := t.In, t.Out
	switch t.Action {
	case string(ActivityBuy):
		a.Kind = ActivityBuy
		total, err := cash.Amount.Add(money.UnitsFromUint64(t.Fee))
		if err != nil {
			return Activity{}, errs.Wrap(err, errs.CodeInvalidInput, "treasury.TradeActivity")
		}
		cash.Amount = total
	case string(ActivitySell):
		a.Kind = ActivitySell
		cash, asset = t.Out, t.In
	default:
		return Activity{}, errs.New(errs.CodeInvalidInput, "treasury.TradeActivity", slog.String("action", t.Action))
	}
	a.Asset = asset.Asset
	micros, err := cash.Amount.Uint64()
	if err != nil {
		return Activity{}, errs.Wrap(err, errs.CodeInvalidInput, "treasury.TradeActivity")
	}
	a.USDCMicros, a.Units = optional(micros), optionalUnits(asset.Amount)
	return a, nil
}

func optional(v uint64) *uint64 {
	if v == 0 {
		return nil
	}
	return &v
}

func optionalUnits(v money.Units) *money.Units {
	if v.IsZero() {
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
		ID:      f.TransferID,
		CabalID: f.CabalID,
		Actor:   f.UserID,
		Kind:    ActivityFund,
		Status:  status,
		USDCMicros: optional(
			f.USDCMicros,
		),
		Units:       optionalUnits(money.UnitsFromUint64(f.ShareUnits)),
		TxSignature: f.TxSignature,
	}
}

type CashOut struct {
	JobID        uuid.UUID
	CabalID      ids.CabalID
	UserID       ids.UserID
	PayoutMicros uint64
	TxSignature  chain.Signature
}

func CashOutActivity(c CashOut, status ActivityStatus) Activity {
	return Activity{
		ID: c.JobID, CabalID: c.CabalID, Actor: c.UserID, Kind: ActivityCashOut, Status: status,
		USDCMicros: optional(c.PayoutMicros), TxSignature: c.TxSignature,
	}
}
