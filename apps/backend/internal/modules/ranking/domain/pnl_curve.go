package domain

import (
	"log/slog"
	"math"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type StakePoint struct {
	CabalID ids.CabalID
	At      time.Time
	Shares  money.SharesUnits
	Net     money.SignedMicros
}

type PnLPoint struct {
	At     time.Time
	Equity money.Micros
	PnL    money.SignedMicros
}

func PnLCurve(r Range, now time.Time, stakes []StakePoint, snaps map[ids.CabalID][]Snapshot) ([]PnLPoint, error) {
	points := []PnLPoint{}
	if len(stakes) == 0 {
		return points, nil
	}
	byCabal := map[ids.CabalID][]StakePoint{}
	for _, s := range stakes {
		byCabal[s.CabalID] = append(byCabal[s.CabalID], s)
	}
	for _, end := range BucketTimes(r, now, stakes[0].At) {
		held, ok, err := heldAt(end, byCabal, snaps)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		pnl, _, err := Lifetime(held.equity, held.net)
		if err != nil {
			return nil, err
		}
		points = append(points, PnLPoint{At: end, Equity: held.equity, PnL: pnl})
	}
	return points, nil
}

type held struct {
	equity money.Micros
	net    money.SignedMicros
	cabals int
}

func heldAt(end time.Time, byCabal map[ids.CabalID][]StakePoint, snaps map[ids.CabalID][]Snapshot) (held, bool, error) {
	var out held
	for cabal, stakes := range byCabal {
		if stakes[0].At.After(end) {
			continue
		}
		snap := LastAtOrBefore(snaps[cabal], end, func(s Snapshot) time.Time { return s.At })
		if snap == nil {
			return held{}, false, nil
		}
		stake := LastAtOrBefore(stakes, snap.At, func(s StakePoint) time.Time { return s.At })
		if stake == nil {
			continue
		}
		if err := out.add(*stake, *snap); err != nil {
			return held{}, false, err
		}
	}
	return out, out.cabals > 0, nil
}

func (h *held) add(stake StakePoint, snap Snapshot) error {
	const op = "ranking.PnLCurve"
	equity, err := MemberEquity(stake.Shares, snap.TotalShares, snap.Value)
	if err != nil {
		return err
	}
	if h.equity, err = h.equity.Add(equity); err != nil {
		return errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	a, b := h.net.Int64(), stake.Net.Int64()
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return errs.New(errs.CodeInvalidInput, op, slog.Int64("net", a), slog.Int64("add", b))
	}
	h.net = money.SignedMicrosFromInt64(a + b)
	h.cabals++
	return nil
}
