package domain

import (
	"log/slog"
	"maps"
	"math/big"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type CabalPositionDelta struct {
	Asset  Asset
	Units  money.SignedMicros
	CostIn money.Micros
}

func (t CabalTxn) PositionDeltas(usdc Asset) ([]CabalPositionDelta, error) {
	const op = "treasury.CabalTxn.PositionDeltas"
	units, paid, err := treasuryUnits(op, t.entries, usdc)
	if err != nil {
		return nil, err
	}
	var bought int
	out := make([]CabalPositionDelta, 0, len(units))
	for _, asset := range slices.Sorted(maps.Keys(units)) {
		v := units[asset]
		if v == 0 {
			continue
		}
		d := CabalPositionDelta{Asset: asset, Units: money.SignedMicrosFromInt64(v)}
		switch {
		case v > 0 && IsCash(asset, usdc):
			d.CostIn = money.MicrosFromUint64(uint64(v))
		case v > 0:
			d.CostIn = paid
			bought++
		}
		out = append(out, d)
	}
	if !paid.IsZero() && bought > 1 {
		return nil, errs.New(errs.CodeInvalidInput, op, slog.Int("bought", bought))
	}
	return out, nil
}

func treasuryUnits(op string, entries []CabalEntry, usdc Asset) (map[Asset]int64, money.Micros, error) {
	sums := map[Asset]*big.Int{}
	spent := new(big.Int)
	for _, e := range entries {
		switch {
		case e.Account == CabalTreasury:
			add(sums, e.Asset, e.Amount)
		case e.Account == CabalFees && e.Amount.Int64() < 0 && IsCash(e.Asset, usdc):
			spent.Sub(spent, big.NewInt(e.Amount.Int64()))
		}
	}
	units := make(map[Asset]int64, len(sums))
	for asset, sum := range sums {
		if !sum.IsInt64() {
			return nil, money.Micros{}, errs.New(errs.CodeInvalidInput, op, slog.String("asset", string(asset)))
		}
		units[asset] = sum.Int64()
	}
	for asset, sum := range sums {
		if sum.Sign() == -1 && IsCash(asset, usdc) {
			spent.Sub(spent, sum)
		}
	}
	if !spent.IsUint64() {
		return nil, money.Micros{}, errs.New(errs.CodeInvalidInput, op, slog.String("paid", spent.String()))
	}
	return units, money.MicrosFromUint64(spent.Uint64()), nil
}

type UserPositionDelta struct {
	Shares      money.SignedMicros
	Contributed money.Micros
	Withdrawn   money.Micros
}

func (t UserTxn) PositionDelta() (UserPositionDelta, error) {
	const op = "treasury.UserTxn.PositionDelta"
	shares := SharesAsset(t.CabalID)
	var units, in, out big.Int
	for _, e := range t.entries {
		v := big.NewInt(e.Amount.Int64())
		switch {
		case e.Account == UserHolder && e.Asset == shares:
			units.Add(&units, v)
		case e.Account == UserCabal && v.Sign() > 0:
			in.Add(&in, v)
		case e.Account == UserCabal:
			out.Sub(&out, v)
		}
	}
	if !units.IsInt64() || !in.IsUint64() || !out.IsUint64() {
		return UserPositionDelta{}, errs.New(errs.CodeInvalidInput, op, slog.String("shares", units.String()))
	}
	return UserPositionDelta{
		Shares:      money.SignedMicrosFromInt64(units.Int64()),
		Contributed: money.MicrosFromUint64(in.Uint64()),
		Withdrawn:   money.MicrosFromUint64(out.Uint64()),
	}, nil
}

func add(sums map[Asset]*big.Int, asset Asset, amount money.SignedMicros) {
	if sums[asset] == nil {
		sums[asset] = new(big.Int)
	}
	sums[asset].Add(sums[asset], big.NewInt(amount.Int64()))
}
