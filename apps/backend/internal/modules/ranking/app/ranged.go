package app

import (
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Snapshot struct {
	Value       money.Micros
	TotalShares money.SharesUnits
}

type MemberKey struct {
	UserID  ids.UserID
	CabalID ids.CabalID
}

type Ranged struct {
	T0, T1     time.Time
	Start, End money.Micros
	Flows      []domain.Flow
}

func (r Ranged) Gain() (money.SignedMicros, *domain.Bps, error) {
	return domain.ModifiedDietz(domain.DietzInput{T0: r.T0, T1: r.T1, Start: r.Start, End: r.End, Flows: r.Flows})
}

func RangeFlows(
	flows []treasury.MemberFlow,
	t0, t1 time.Time,
) (map[MemberKey][]domain.Flow, map[ids.CabalID][]domain.Flow) {
	byMember := map[MemberKey][]domain.Flow{}
	byCabal := map[ids.CabalID][]domain.Flow{}
	for _, f := range flows {
		if !f.At.After(t0) || f.At.After(t1) {
			continue
		}
		flow := domain.Flow{Amount: f.Amount, At: f.At}
		key := MemberKey{UserID: f.UserID, CabalID: f.CabalID}
		byMember[key] = append(byMember[key], flow)
		byCabal[f.CabalID] = append(byCabal[f.CabalID], flow)
	}
	return byMember, byCabal
}

func MemberRanged(
	t0, t1 time.Time,
	shares money.SharesUnits,
	snap *Snapshot,
	end money.Micros,
	flows []domain.Flow,
) (Ranged, error) {
	r := Ranged{T0: t0, T1: t1, End: end, Flows: flows}
	if snap == nil {
		return r, nil
	}
	start, err := domain.MemberEquity(shares, snap.TotalShares, snap.Value)
	r.Start = start
	return r, err
}

func CabalRanged(t0, t1 time.Time, snap *Snapshot, end money.Micros, flows []domain.Flow) Ranged {
	r := Ranged{T0: t0, T1: t1, End: end, Flows: flows}
	if snap != nil {
		r.Start = snap.Value
	}
	return r
}

func SumRanged(t0, t1 time.Time, parts []Ranged) (Ranged, error) {
	sum := Ranged{T0: t0, T1: t1}
	for _, p := range parts {
		var err error
		if sum.Start, err = sum.Start.Add(p.Start); err != nil {
			return Ranged{}, err
		}
		if sum.End, err = sum.End.Add(p.End); err != nil {
			return Ranged{}, err
		}
		sum.Flows = append(sum.Flows, p.Flows...)
	}
	return sum, nil
}
