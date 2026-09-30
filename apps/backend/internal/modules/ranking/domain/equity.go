package domain

import (
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func MemberEquity(shares, total money.SharesUnits, nav money.Micros) (money.Micros, error) {
	if shares.IsZero() {
		return money.Micros{}, nil
	}
	if shares.Uint64() > total.Uint64() {
		return money.Micros{}, errs.New(errs.CodeInvalidInput, "ranking.MemberEquity",
			slog.String("shares", shares.String()), slog.String("total", total.String()))
	}
	v, err := money.MulDiv(shares.Uint64(), nav.Uint64(), total.Uint64())
	return money.MicrosFromUint64(v), err
}
