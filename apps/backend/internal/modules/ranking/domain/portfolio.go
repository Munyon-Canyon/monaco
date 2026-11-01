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
	CabalID      ids.CabalID
	PotValue     money.Micros
	Value        money.Micros
	Shares       money.SharesUnits
	Net          money.SignedMicros
	PnL          money.SignedMicros
	Return       *Bps
	SliceBps     uint64
	OwnershipBps int32
}

type Portfolio struct {
	Total  money.Micros
	PnL    money.SignedMicros
	Return *Bps
	Rows   []PortfolioRow
}

func NewPortfolio(
	stakes []StakePoint, latest map[ids.CabalID]Snapshot, memberships []ids.CabalID, skipped func(ids.CabalID),
) (Portfolio, error) {
	byCabal := map[ids.CabalID][]StakePoint{}
	for _, s := range stakes {
		byCabal[s.CabalID] = append(byCabal[s.CabalID], s)
	}
	out := Portfolio{Rows: []PortfolioRow{}}
	exited, err := out.collect(byCabal, latest, skipped)
	if err != nil {
		return Portfolio{}, err
	}
	net, err := out.sum()
	if err != nil {
		return Portfolio{}, err
	}
	if net, err = addSigned("ranking.NewPortfolio", net, exited); err != nil {
		return Portfolio{}, err
	}
	slices.SortFunc(out.Rows, func(a, b PortfolioRow) int {
		return cmp.Or(b.Value.Cmp(a.Value), cmp.Compare(a.CabalID.String(), b.CabalID.String()))
	})
	out.slice()
	out.PnL, out.Return, err = Lifetime(out.Total, net)
	if err != nil {
		return Portfolio{}, err
	}
	out.Rows = append(out.Rows, unfundedRows(out.Rows, latest, memberships)...)
	inOrder(out.Rows, memberships)
	return out, nil
}

func (p *Portfolio) collect(
	byCabal map[ids.CabalID][]StakePoint, latest map[ids.CabalID]Snapshot, skipped func(ids.CabalID),
) (money.SignedMicros, error) {
	var exited money.SignedMicros
	for cabal, points := range byCabal {
		snap, ok := latest[cabal]
		if !ok {
			continue
		}
		stake := LastAtOrBefore(points, snap.At, func(s StakePoint) time.Time { return s.At })
		if stake == nil {
			continue
		}
		if stake.Shares.IsZero() {
			var err error
			if exited, err = addSigned("ranking.NewPortfolio", exited, stake.Net); err != nil {
				return money.SignedMicros{}, err
			}
			continue
		}
		row, err := rowOf(*stake, snap)
		if errors.Is(err, ErrUnusableStart) {
			skipped(cabal)
			continue
		}
		if err != nil {
			return money.SignedMicros{}, err
		}
		p.Rows = append(p.Rows, row)
	}
	return exited, nil
}

func unfundedRows(valued []PortfolioRow, latest map[ids.CabalID]Snapshot, memberships []ids.CabalID) []PortfolioRow {
	held := make(map[ids.CabalID]bool, len(valued))
	for _, row := range valued {
		held[row.CabalID] = true
	}
	rows := []PortfolioRow{}
	for _, cabal := range memberships {
		if held[cabal] {
			continue
		}
		held[cabal] = true
		rows = append(rows, PortfolioRow{CabalID: cabal, PotValue: latest[cabal].Value})
	}
	return rows
}

func inOrder(rows []PortfolioRow, memberships []ids.CabalID) {
	rank := make(map[ids.CabalID]int, len(memberships))
	for i, cabal := range memberships {
		if _, ok := rank[cabal]; !ok {
			rank[cabal] = i
		}
	}
	at := func(row PortfolioRow) int {
		if i, ok := rank[row.CabalID]; ok {
			return i
		}
		return len(memberships)
	}
	slices.SortStableFunc(rows, func(a, b PortfolioRow) int { return cmp.Compare(at(a), at(b)) })
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
		CabalID: stake.CabalID, PotValue: snap.Value, Value: equity, Shares: stake.Shares, Net: stake.Net,
		PnL: pnl, Return: ret,
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
