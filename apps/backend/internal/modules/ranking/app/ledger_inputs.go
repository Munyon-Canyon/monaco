package app

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	ledgerSeriesStep   = 4 * time.Hour
	ledgerSeriesPoints = int(domain.LedgerWindow / ledgerSeriesStep)
	ledgerBpsScale     = 10_000
)

type LedgerWindow struct {
	t0, t1 time.Time
	start  rangedBook
	end    rangedBook
	flows  map[MemberKey][]domain.Flow
}

func NewLedgerWindow(ctx context.Context, ports Ports, t0, t1 time.Time) (*LedgerWindow, error) {
	books := []rangedBook{newRangedBook(t0, t1), newRangedBook(t0, t1)}
	for i, at := range []time.Time{t0, t1} {
		stakes, err := ports.Treasury.MemberStakesAt(ctx, at)
		if err != nil {
			return nil, err
		}
		if err := books[i].addStakes(stakes); err != nil {
			return nil, err
		}
	}
	rows, err := ports.Snapshots.SnapshotsAt(ctx, []time.Time{t0, t1})
	if err != nil {
		return nil, err
	}
	if err := addSnapshots(books, rows); err != nil {
		return nil, err
	}
	flows, err := ports.Treasury.MemberFlowsBetween(ctx, t0, t1)
	if err != nil {
		return nil, err
	}
	byMember, _ := RangeFlows(flows, t0, t1)
	return &LedgerWindow{t0: t0, t1: t1, start: books[0], end: books[1], flows: byMember}, nil
}

func newRangedBook(t0, t1 time.Time) rangedBook {
	return rangedBook{
		Window: Window{Range: domain.Range1W, T0: t0, T1: t1}, stakes: map[MemberKey]money.SharesUnits{},
		shares: map[ids.CabalID]money.SharesUnits{}, users: map[ids.CabalID][]ids.UserID{},
		snaps: map[ids.CabalID]*Snapshot{},
	}
}

type heldCabal struct {
	id     ids.CabalID
	equity money.Micros
}

type memberWindow struct {
	cabals    []heldCabal
	flows     []domain.Flow
	returnBps *int64
}

func (w *LedgerWindow) member(user ids.UserID) (memberWindow, error) {
	keys := map[MemberKey]bool{}
	for key := range w.start.stakes {
		keys[key] = true
	}
	for key := range w.end.stakes {
		keys[key] = true
	}
	for key := range w.flows {
		keys[key] = true
	}
	out := memberWindow{}
	parts := []Ranged{}
	usable := true
	for key := range keys {
		if key.UserID != user {
			continue
		}
		equity, part, err := w.part(key)
		unusable := errors.Is(err, domain.ErrUnusableStart)
		if err != nil && !unusable {
			return memberWindow{}, err
		}
		usable = usable && !unusable
		parts = append(parts, part)
		out.flows = append(out.flows, w.flows[key]...)
		out.cabals = append(out.cabals, heldCabal{id: key.CabalID, equity: equity})
	}
	if usable && len(parts) > 0 {
		bps, err := w.gain(parts)
		if err != nil {
			return memberWindow{}, err
		}
		out.returnBps = bps
	}
	return out, nil
}

func (w *LedgerWindow) gain(parts []Ranged) (*int64, error) {
	sum, err := SumRanged(w.t0, w.t1, parts)
	if err != nil {
		return nil, err
	}
	_, bps, err := sum.Gain()
	if err != nil || bps == nil {
		return nil, err
	}
	v := int64(*bps)
	return &v, nil
}

func (w *LedgerWindow) endEquity(key MemberKey) (money.Micros, error) {
	shares := w.end.stakes[key]
	if shares.IsZero() {
		return money.Micros{}, nil
	}
	snap := w.end.snaps[key.CabalID]
	end := Window{Range: domain.Range1W, T0: w.t1}
	if !end.usable(snap, w.end.shares[key.CabalID]) {
		return money.Micros{}, end.unusable("LedgerWindow")
	}
	return domain.MemberEquity(shares, snap.TotalShares, snap.Value)
}

func (w *LedgerWindow) part(key MemberKey) (money.Micros, Ranged, error) {
	equity, err := w.endEquity(key)
	if err != nil {
		return money.Micros{}, Ranged{}, err
	}
	part, err := MemberRanged(
		w.start.Window, w.start.shares[key.CabalID], w.start.stakes[key], w.start.snaps[key.CabalID], equity,
		w.flows[key],
	)
	return equity, part, err
}

func (m memberWindow) ids() []ids.CabalID {
	out := make([]ids.CabalID, len(m.cabals))
	for i, h := range m.cabals {
		out[i] = h.id
	}
	return out
}

func (in LedgerInputs) series(
	ctx context.Context, user ids.UserID, w *LedgerWindow, m memberWindow,
) ([]domain.LedgerPoint, error) {
	out := []domain.LedgerPoint{}
	history, err := in.Stakes.UserStakeHistory(ctx, user)
	if err != nil || len(history) == 0 {
		return out, err
	}
	points, cabals := stakePoints(history)
	snaps, err := in.Snaps.SnapshotsOfCabals(ctx, cabals, w.t0, w.t1)
	if err != nil {
		return nil, err
	}
	curve, err := domain.PnLCurve(domain.Range1W, w.t1, points, snaps, logSkipped(ctx))
	if err != nil {
		return nil, err
	}
	equities := make(map[time.Time]money.Micros, len(curve))
	for _, p := range curve {
		equities[p.At] = p.Equity
	}
	for i := 1; i <= ledgerSeriesPoints; i++ {
		at := w.t0.Add(time.Duration(i) * ledgerSeriesStep)
		bps, err := seriesReturn(w.t0, at, equities, m.flows)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.LedgerPoint{At: at, ReturnBps: bps})
	}
	return out, nil
}

func seriesReturn(t0, at time.Time, equities map[time.Time]money.Micros, all []domain.Flow) (int64, error) {
	flows := slices.DeleteFunc(slices.Clone(all), func(f domain.Flow) bool { return f.At.After(at) })
	r := Ranged{T0: t0, T1: at, Start: equities[t0], End: equities[at], Flows: flows}
	_, bps, err := r.Gain()
	if err != nil || bps == nil {
		return 0, err
	}
	return int64(*bps), nil
}
