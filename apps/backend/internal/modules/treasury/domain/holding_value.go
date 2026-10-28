package domain

import (
	"math/big"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func HoldingValue(units, price, multiplier, scale, divisor uint64) (uint64, error) {
	numerator := new(big.Int).SetUint64(units)
	numerator.Mul(numerator, new(big.Int).SetUint64(price))
	numerator.Mul(numerator, new(big.Int).SetUint64(multiplier))
	denominator := new(big.Int).SetUint64(scale)
	denominator.Mul(denominator, new(big.Int).SetUint64(divisor))
	numerator.Quo(numerator, denominator)
	if !numerator.IsUint64() {
		return 0, errs.New(errs.CodeInvalidInput, "treasury.HoldingValue")
	}
	return numerator.Uint64(), nil
}
