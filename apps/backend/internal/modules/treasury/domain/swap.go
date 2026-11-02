package domain

import (
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Leg struct {
	Asset  Asset
	Amount money.Units
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
		{CabalTreasury, CabalFees, Leg{Asset: usdc, Amount: money.UnitsFromMicros(s.Fee)}},
	}
	if s.In.Asset != usdc && IsCash(s.In.Asset, usdc) {
		legs[2].from, legs[2].to, legs[2].leg.Asset = CabalFees, CabalVenue, s.In.Asset
	}
	var entries []CabalEntry
	for _, l := range legs {
		if l.leg.Amount.IsZero() {
			continue
		}
		if IsCash(l.leg.Asset, usdc) {
			if _, err := l.leg.Amount.Int64(); err != nil {
				return CabalTxn{}, errs.Wrap(err, errs.CodeInvalidInput, "treasury.NewSwapTxn",
					slog.String("asset", string(l.leg.Asset)))
			}
		}
		entries = append(entries,
			CabalEntry{Account: l.from, Asset: l.leg.Asset, Amount: l.leg.Amount.Signed().Neg()},
			CabalEntry{Account: l.to, Asset: l.leg.Asset, Amount: l.leg.Amount.Signed()})
	}
	return NewCabalTxn(CabalTxnHeader{
		ID: id, CabalID: s.CabalID, Kind: CabalSwap, Status: TxnSettled, SwapID: s.ID, TxSignature: s.TxSignature,
	}, entries)
}
