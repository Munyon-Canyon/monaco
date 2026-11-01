package domain

import (
	"errors"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type CashWriteOff struct {
	SwapID  uuid.UUID
	CabalID ids.CabalID
	Network chain.Network
	Amount  money.Micros
}

func (w CashWriteOff) TxnID() uuid.UUID {
	return uuid.NewSHA1(w.SwapID, []byte("cash_write_off:"+string(w.Network)))
}

func NewCashWriteOffTxn(w CashWriteOff) (CabalTxn, error) {
	debit, debitErr := money.Micros{}.Delta(w.Amount)
	credit, creditErr := w.Amount.Delta(money.Micros{})
	if err := errors.Join(debitErr, creditErr); err != nil {
		return CabalTxn{}, err
	}
	asset := EVMCashAsset(w.Network)
	return NewCabalTxn(CabalTxnHeader{ID: w.TxnID(), CabalID: w.CabalID, Kind: CabalSwap, Status: TxnSettled},
		[]CabalEntry{
			{Account: CabalTreasury, Asset: asset, Amount: debit},
			{Account: CabalFees, Asset: asset, Amount: credit},
		})
}
