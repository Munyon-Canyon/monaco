package adapters

import (
	"context"
	"maps"
	"math/big"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
)

type costBasis struct {
	units, replayed, stored big.Int
}

type costBases map[string]*costBasis

func (c costBases) at(key string) *costBasis {
	if c[key] == nil {
		c[key] = &costBasis{}
	}
	return c[key]
}

func (b *costBasis) apply(units, costIn *big.Int) {
	if units.Sign() < 0 {
		divisor := &b.units
		if b.units.Sign() <= 0 {
			divisor = big.NewInt(1)
		}
		released := new(big.Int).Mul(&b.replayed, new(big.Int).Neg(units))
		b.replayed.Sub(&b.replayed, released.Quo(released, divisor))
	} else {
		b.replayed.Add(&b.replayed, costIn)
	}
	b.units.Add(&b.units, units)
}

func integer(text string) *big.Int {
	v, _ := new(big.Int).SetString(text, 10)
	return v
}

func costBasisDrift(ctx context.Context, q *sqlc.Queries, usdc domain.Asset) ([]string, error) {
	const op = "treasury.CheckLedger"
	rows, err := q.CabalCosts(ctx, string(usdc))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	bases := costBases{}
	for _, r := range rows {
		b := bases.at(r.CabalID + " " + r.Asset)
		if r.Stored {
			b.stored.Set(integer(r.Cost))
		} else {
			b.apply(integer(r.Units), integer(r.Cost))
		}
	}
	var out []string
	for _, key := range slices.Sorted(maps.Keys(bases)) {
		if b := bases[key]; b.replayed.Cmp(&b.stored) != 0 {
			out = append(out, "cabal_positions "+key+": cost basis entries "+b.replayed.String()+
				", position "+b.stored.String())
		}
	}
	return out, nil
}
