package domain

import (
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Snapshot struct {
	At          time.Time
	Value       money.Micros
	NavPerShare money.Micros
	TotalShares money.SharesUnits
}

type Contribution struct {
	At  time.Time
	Net money.SignedMicros
}

type ValuePoint struct {
	At          time.Time
	Value       money.Micros
	NavPerShare money.Micros
	PnL         money.SignedMicros
}

type ValueHistory struct {
	Points     []ValuePoint
	PricesAsOf *time.Time
}

func ValueCurve(r Range, now time.Time, snaps []Snapshot, contributed []Contribution) ([]ValuePoint, error) {
	points := []ValuePoint{}
	if len(snaps) == 0 {
		return points, nil
	}
	for _, end := range BucketTimes(r, now, snaps[0].At) {
		snap := LastAtOrBefore(snaps, end, func(s Snapshot) time.Time { return s.At })
		if snap == nil {
			continue
		}
		pnl, _, err := PotLifetime(*snap, contributed)
		if err != nil {
			return nil, err
		}
		points = append(points, ValuePoint{At: end, Value: snap.Value, NavPerShare: snap.NavPerShare, PnL: pnl})
	}
	return points, nil
}

func PotLifetime(snap Snapshot, contributed []Contribution) (money.SignedMicros, *Bps, error) {
	var net money.SignedMicros
	if c := LastAtOrBefore(contributed, snap.At, func(c Contribution) time.Time { return c.At }); c != nil {
		net = c.Net
	}
	return Lifetime(snap.Value, net)
}
