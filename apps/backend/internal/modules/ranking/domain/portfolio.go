package domain

import (
	"cmp"
	"errors"
	"log/slog"
	"math/bits"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type PortfolioRow struct {
	CabalID  ids.CabalID
	Value    money.Micros
	Shares   money.SharesUnits
	Net      money.SignedMicros
	PnL      money.SignedMicros
	Return   *Bps
	SliceBps uint64
}

type Portfolio struct {
	Total  money.Micros
	PnL    money.SignedMicros
	Return *Bps
	Rows   []PortfolioRow
}

func NewPortfolio(stakes []StakePoint, latest map[ids.CabalID]Snapshot, skipped func(ids.CabalID)) (Portfolio, error) {
	byCabal := map[ids.CabalID][]StakePoint{}
	for _, s := range stakes {
		byCabal[s.CabalID] = append(byCabal[s.CabalID], s)
	}
	out := Portfolio{Rows: []PortfolioRow{}}
	for cabal, points := range byCabal {
		snap, ok := latest[cabal]
		if !ok {
			continue
		}
		stake := LastAtOrBefore(points, snap.At, func(s StakePoint) time.Time { return s.At })
		if stake == nil || stake.Shares.IsZero() {
			continue
		}
		row, err := rowOf(*stake, snap)
		if errors.Is(err, ErrUnusableStart) {
			skipped(cabal)
			continue
		}
		if err != nil {
			return Portfolio{}, err
		}
		out.Rows = append(out.Rows, row)
	}
	net, err := out.sum()
	if err != nil {
		return Portfolio{}, err
	}
	slices.SortFunc(out.Rows, func(a, b PortfolioRow) int {
		return cmp.Or(b.Value.Cmp(a.Value), cmp.Compare(a.CabalID.String(), b.CabalID.String()))
	})
	out.slice()
	out.PnL, out.Return, err = Lifetime(out.Total, net)
	return out, err
}

func (p *Portfolio) sum() (money.SignedMicros, error) {
	const op = "ranking.NewPortfolio"
	var net money.SignedMicros
	for _, row := range p.Rows {
		var err error
		if p.Total, err = p.Total.Add(row.Value); err != nil {
			return money.SignedMicros{}, err
		}
		if net, err = addSigned(op, net, row.Net); err != nil {
			return money.SignedMicros{}, err
		}
	}
	return net, nil
}

func rowOf(stake StakePoint, snap Snapshot) (PortfolioRow, error) {
	equity, err := MemberEquity(stake.Shares, snap.TotalShares, snap.Value)
	if err != nil {
		return PortfolioRow{}, err
	}
	pnl, ret, err := Lifetime(equity, stake.Net)
	return PortfolioRow{
		CabalID: stake.CabalID, Value: equity, Shares: stake.Shares, Net: stake.Net, PnL: pnl, Return: ret,
	}, err
}

func (p *Portfolio) slice() {
	values := make([]money.Micros, len(p.Rows))
	for i, h := range p.Rows {
		values[i] = h.Value
	}
	for i, b := range slicesOf(values, p.Total) {
		p.Rows[i].SliceBps = b
	}
}

func Slices(values []money.Micros, total money.Micros) ([]uint64, error) {
	for _, v := range values {
		if v.Cmp(total) > 0 {
			return nil, errs.New(errs.CodeInvalidInput, "ranking.Slices",
				slog.String("value", v.String()), slog.String("total", total.String()))
		}
	}
	return slicesOf(values, total), nil
}

func slicesOf(values []money.Micros, total money.Micros) []uint64 {
	out := make([]uint64, len(values))
	if total.IsZero() {
		return out
	}
	var given uint64
	largest := 0
	for i, v := range values {
		hi, lo := bits.Mul64(v.Uint64(), bpsScale)
		floor, _ := bits.Div64(hi, lo, total.Uint64())
		out[i] = floor
		given += floor
		if v.Cmp(values[largest]) > 0 {
			largest = i
		}
	}
	out[largest] += bpsScale - given
	return out
}
