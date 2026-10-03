package domain

import (
	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Leg struct {
	Asset  Asset
	Amount uint64
}

type Swap struct {
	ID          uuid.UUID
	CabalID     ids.CabalID
	In          Leg
	Out         Leg
	Fee         money.Micros
	TxSignature chain.Signature
}

func NewSwapTxn(id uuid.UUID, s Swap, usdc Asset) (CabalTxn, error) {
	legs := []struct {
		from, to CabalAccount
		leg      Leg
	}{
		{CabalTreasury, CabalVenue, s.In},
		{CabalVenue, CabalTreasury, s.Out},
		{CabalTreasury, CabalFees, Leg{Asset: usdc, Amount: s.Fee.Uint64()}},
	}
	var entries []CabalEntry
	for _, l := range legs {
		if l.leg.Amount == 0 {
			continue
		}
		amount := money.MicrosFromUint64(l.leg.Amount)
		debit, err := money.Micros{}.Delta(amount)
		if err != nil {
			return CabalTxn{}, err
		}
		credit, err := amount.Delta(money.Micros{})
		if err != nil {
			return CabalTxn{}, err
		}
		entries = append(entries,
			CabalEntry{Account: l.from, Asset: l.leg.Asset, Amount: debit},
			CabalEntry{Account: l.to, Asset: l.leg.Asset, Amount: credit})
	}
	return NewCabalTxn(CabalTxnHeader{
		ID: id, CabalID: s.CabalID, Kind: CabalSwap, Status: TxnSettled, SwapID: s.ID, TxSignature: s.TxSignature,
	}, entries)
}
