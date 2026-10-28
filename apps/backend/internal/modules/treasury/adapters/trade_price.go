package adapters

import (
	"log/slog"
	"math/bits"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const maxWholeDecimals = 19

func tradePriceMicros(usdc money.Micros, units money.BaseUnits, num, den int64) (money.Micros, bool, error) {
	const op = "treasury.tradePriceMicros"
	if units.IsZero() {
		return money.Micros{}, false, nil
	}
	if num <= 0 || den <= 0 || units.Decimals() > maxWholeDecimals {
		return money.Micros{}, false, errs.New(errs.CodeInvalidInput, op,
			slog.Int64("num", num), slog.Int64("den", den), slog.Int("decimals", int(units.Decimals())))
	}
	scaleHi, scale := bits.Mul64(money.OneWhole(units.Decimals()), uint64(den))
	sharesHi, shares := bits.Mul64(units.Uint64(), uint64(num))
	if scaleHi != 0 || sharesHi != 0 {
		return money.Micros{}, false, errs.New(errs.CodeInvalidInput, op,
			slog.String("units", units.String()), slog.Int64("num", num), slog.Int64("den", den))
	}
	price, err := money.MulDiv(usdc.Uint64(), scale, shares)
	if err != nil {
		return money.Micros{}, false, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	return money.MicrosFromUint64(price), true, nil
}
