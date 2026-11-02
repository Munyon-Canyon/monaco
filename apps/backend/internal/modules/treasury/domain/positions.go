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
	Units  money.SignedUnits
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
		if v.Sign() == 0 {
			continue
		}
		signed, err := money.SignedUnitsFromBig(v)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInvalidInput, op, slog.String("asset", string(asset)))
		}
		d := CabalPositionDelta{Asset: asset, Units: signed}
		switch {
		case v.Sign() > 0 && IsCash(asset, usdc):
			if !v.IsUint64() {
				return nil, errs.New(errs.CodeInvalidInput, op, slog.String("asset", string(asset)))
			}
			d.CostIn = money.MicrosFromUint64(v.Uint64())
		case v.Sign() > 0:
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

func treasuryUnits(op string, entries []CabalEntry, usdc Asset) (map[Asset]*big.Int, money.Micros, error) {
	sums := map[Asset]*big.Int{}
	spent := new(big.Int)
	for _, e := range entries {
		amount := e.Amount.Big()
		switch {
		case e.Account == CabalTreasury:
			addBig(sums, e.Asset, amount)
		case e.Account == CabalFees && amount.Sign() < 0 && IsCash(e.Asset, usdc):
			spent.Sub(spent, amount)
		}
	}
	for asset, sum := range sums {
		if sum.Sign() == -1 && IsCash(asset, usdc) {
			spent.Sub(spent, sum)
		}
	}
	if !spent.IsUint64() {
		return nil, money.Micros{}, errs.New(errs.CodeInvalidInput, op, slog.String("paid", spent.String()))
	}
	return sums, money.MicrosFromUint64(spent.Uint64()), nil
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

func addBig(sums map[Asset]*big.Int, asset Asset, amount *big.Int) {
	if sums[asset] == nil {
		sums[asset] = new(big.Int)
	}
	sums[asset].Add(sums[asset], amount)
}
