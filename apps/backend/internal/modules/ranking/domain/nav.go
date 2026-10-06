package domain

import (
	"log/slog"
	"math/big"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const shareScale = 1_000_000

type Holding struct {
	AssetID string
	Units   money.BaseUnits
	Price   money.Micros
}

type NAVInput struct {
	USDC        money.Micros
	Reserved    money.Micros
	Holdings    []Holding
	TotalShares money.SharesUnits
}

type NAV struct {
	Value    money.Micros
	PerShare money.Micros
}

func CabalNAV(in NAVInput) (NAV, error) {
	value := in.USDC
	for _, h := range in.Holdings {
		worth, err := holdingValue(h)
		if err != nil {
			return NAV{}, err
		}
		if value, err = value.Add(worth); err != nil {
			return NAV{}, err
		}
	}
	available, err := value.Sub(in.Reserved)
	if err != nil {
		return NAV{}, errs.New(errs.CodeConservationBroken, "ranking.CabalNAV",
			slog.String("value", value.String()), slog.String("reserved", in.Reserved.String()))
	}
	if in.TotalShares.IsZero() {
		return NAV{Value: available}, nil
	}
	perShare, err := money.MulDiv(available.Uint64(), shareScale, in.TotalShares.Uint64())
	if err != nil {
		return NAV{}, err
	}
	return NAV{Value: available, PerShare: money.MicrosFromUint64(perShare)}, nil
}

func holdingValue(h Holding) (money.Micros, error) {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(h.Units.Decimals())), nil)
	if !scale.IsUint64() {
		return money.Micros{}, errs.New(errs.CodeInvalidInput, "ranking.CabalNAV",
			slog.String("asset_id", h.AssetID), slog.Int("decimals", int(h.Units.Decimals())))
	}
	v, err := money.MulDiv(h.Units.Uint64(), h.Price.Uint64(), scale.Uint64())
	if err != nil {
		return money.Micros{}, err
	}
	return money.MicrosFromUint64(v), nil
}
