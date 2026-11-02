package domain

import (
	"cmp"
	"math/big"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const CashOutBufferBps = 100

type Lot struct {
	Mint      chain.Mint
	Symbol    string
	Units     money.Units
	QuoteUSDC uint64
	Listing   *multichain.Listing
}

func PlanCashOutSell(quoted []Lot, needed money.Micros) ([]Lot, error) {
	remaining, err := money.MulDiv(needed.Uint64(), 10_000+CashOutBufferBps, 10_000)
	if err != nil {
		return nil, err
	}
	ranked := slices.DeleteFunc(slices.Clone(quoted), func(l Lot) bool { return l.Units.IsZero() || l.QuoteUSDC == 0 })
	slices.SortFunc(ranked, func(a, b Lot) int {
		return cmp.Or(cmp.Compare(b.QuoteUSDC, a.QuoteUSDC), cmp.Compare(a.Mint.Address, b.Mint.Address))
	})
	plan := []Lot{}
	for _, lot := range ranked {
		if remaining == 0 {
			break
		}
		if lot.QuoteUSDC > remaining {
			part, err := lot.part(remaining)
			return append(plan, part), err
		}
		plan = append(plan, lot)
		remaining -= lot.QuoteUSDC
	}
	return plan, nil
}

func (l Lot) part(usdc uint64) (Lot, error) {
	den := new(big.Int).SetUint64(l.QuoteUSDC)
	num := new(big.Int).Mul(l.Units.Big(), new(big.Int).SetUint64(usdc))
	num.Add(num, new(big.Int).Sub(den, big.NewInt(1)))
	units, err := money.UnitsFromBig(num.Quo(num, den))
	if err != nil {
		return Lot{}, err
	}
	if units.Cmp(l.Units) > 0 {
		units = l.Units
	}
	quote := new(big.Int).Mul(den, units.Big())
	quote.Quo(quote, l.Units.Big())
	return Lot{Mint: l.Mint, Symbol: l.Symbol, Units: units, QuoteUSDC: quote.Uint64(), Listing: l.Listing}, nil
}
