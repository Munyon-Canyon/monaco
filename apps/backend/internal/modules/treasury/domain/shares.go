package domain

import (
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func MintShares(in money.Micros, total money.SharesUnits, pot money.Micros) (money.SharesUnits, error) {
	const op = "treasury.MintShares"
	if total.IsZero() {
		return money.SharesUnitsFromUint64(in.Uint64()), nil
	}
	if pot.IsZero() {
		return money.SharesUnits{}, errs.New(errs.CodePotValueZero, op, slog.String("total", total.String()))
	}
	units, err := money.MulDiv(in.Uint64(), total.Uint64(), pot.Uint64())
	if err != nil {
		return money.SharesUnits{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	return money.SharesUnitsFromUint64(units), nil
}

func PayoutFor(units, total money.SharesUnits, pot money.Micros) (money.Micros, error) {
	payout, err := money.MulDiv(units.Uint64(), pot.Uint64(), total.Uint64())
	if err != nil {
		return money.Micros{}, errs.Wrap(err, errs.CodeOf(err), "treasury.PayoutFor")
	}
	return money.MicrosFromUint64(payout), nil
}
