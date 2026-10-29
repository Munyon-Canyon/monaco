package domain

import (
	"log/slog"
	"math/big"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const maxWholeDecimals = 19

func TradePriceMicros(usdc money.Micros, units money.BaseUnits, num, den int64) (money.Micros, bool, error) {
	const op = "treasury.TradePriceMicros"
	if units.IsZero() {
		return money.Micros{}, false, nil
	}
	if num <= 0 || den <= 0 || units.Decimals() > maxWholeDecimals {
		return money.Micros{}, false, errs.New(errs.CodeInvalidInput, op,
			slog.Int64("num", num), slog.Int64("den", den), slog.Int("decimals", int(units.Decimals())))
	}
	numerator := new(big.Int).SetUint64(usdc.Uint64())
	numerator.Mul(numerator, new(big.Int).SetUint64(money.OneWhole(units.Decimals())))
	numerator.Mul(numerator, big.NewInt(den))
	denominator := new(big.Int).SetUint64(units.Uint64())
	denominator.Mul(denominator, big.NewInt(num))
	price := numerator.Quo(numerator, denominator)
	if !price.IsUint64() {
		return money.Micros{}, false, errs.New(errs.CodeInvalidInput, op,
			slog.String("units", units.String()), slog.Int64("num", num), slog.Int64("den", den))
	}
	return money.MicrosFromUint64(price.Uint64()), true, nil
}
