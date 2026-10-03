package domain

import (
	"cmp"
	"math/bits"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const CashOutBufferBps = 100

type Lot struct {
	Mint      chain.Mint
	Symbol    string
	Units     uint64
	QuoteUSDC uint64
}

func PlanCashOutSell(quoted []Lot, needed money.Micros) ([]Lot, error) {
	remaining, err := money.MulDiv(needed.Uint64(), 10_000+CashOutBufferBps, 10_000)
	if err != nil {
		return nil, err
	}
	ranked := slices.DeleteFunc(slices.Clone(quoted), func(l Lot) bool { return l.Units == 0 || l.QuoteUSDC == 0 })
	slices.SortFunc(ranked, func(a, b Lot) int {
		return cmp.Or(cmp.Compare(b.QuoteUSDC, a.QuoteUSDC), cmp.Compare(a.Mint.Address, b.Mint.Address))
	})
	plan := []Lot{}
	for _, lot := range ranked {
		if remaining == 0 {
			break
		}
		if lot.QuoteUSDC > remaining {
			return append(plan, lot.part(remaining)), nil
		}
		plan = append(plan, lot)
		remaining -= lot.QuoteUSDC
	}
	return plan, nil
}

func (l Lot) part(usdc uint64) Lot {
	hi, lo := bits.Mul64(l.Units, usdc)
	lo, carry := bits.Add64(lo, l.QuoteUSDC-1, 0)
	units, _ := bits.Div64(hi+carry, lo, l.QuoteUSDC)
	units = min(units, l.Units)
	hi, lo = bits.Mul64(l.QuoteUSDC, units)
	quote, _ := bits.Div64(hi, lo, l.Units)
	return Lot{Mint: l.Mint, Symbol: l.Symbol, Units: units, QuoteUSDC: quote}
}
