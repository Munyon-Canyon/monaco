package domain

import (
	"log/slog"
	"math/big"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func CheckConservation(nav money.Micros, equities []money.Micros) error {
	sum := new(big.Int)
	for _, e := range equities {
		sum.Add(sum, new(big.Int).SetUint64(e.Uint64()))
	}
	shortfall := new(big.Int).Sub(new(big.Int).SetUint64(nav.Uint64()), sum)
	if shortfall.Sign() >= 0 && shortfall.Cmp(big.NewInt(int64(len(equities)))) <= 0 {
		return nil
	}
	return errs.New(errs.CodeConservationBroken, "ranking.CheckConservation",
		slog.String("nav", nav.String()), slog.String("equity_sum", sum.String()), slog.Int("members", len(equities)))
}
