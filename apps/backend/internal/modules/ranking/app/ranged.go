package app

import (
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Snapshot struct {
	At          time.Time
	Value       money.Micros
	TotalShares money.SharesUnits
}

type Window struct {
	Range  domain.Range
	T0, T1 time.Time
}

func (w Window) usable(snap *Snapshot, ledgerTotal money.SharesUnits) bool {
	return snap != nil && !snap.At.After(w.T0) && w.T0.Sub(snap.At) <= domain.MaxSnapshotGap(w.Range) &&
		snap.TotalShares.Cmp(ledgerTotal) == 0
}

func (w Window) valid() error {
	if !w.T1.After(w.T0) {
		return errs.New(errs.CodeInvalidInput, "ranking.Window",
			slog.String("range", string(w.Range)), slog.Time("t0", w.T0), slog.Time("t1", w.T1))
	}
	return nil
}

func (w Window) unusable(subject string) error {
	return errs.Wrap(domain.ErrUnusableStart, errs.CodeInvalidInput, "ranking."+subject,
		slog.String("range", string(w.Range)), slog.Time("t0", w.T0))
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
	if r.Start.IsZero() && len(r.Flows) == 0 {
		return money.SignedMicros{}, nil, nil
	}
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
	w Window,
	cabalShares, shares money.SharesUnits,
	snap *Snapshot,
	end money.Micros,
	flows []domain.Flow,
) (Ranged, error) {
	if err := w.valid(); err != nil {
		return Ranged{}, err
	}
	r := Ranged{T0: w.T0, T1: w.T1, End: end, Flows: flows}
	if shares.IsZero() {
		return r, nil
	}
	if !w.usable(snap, cabalShares) {
		return Ranged{}, w.unusable("MemberRanged")
	}
	start, err := domain.MemberEquity(shares, snap.TotalShares, snap.Value)
	if err != nil {
		return Ranged{}, err
	}
	r.Start = start
	return r, nil
}

func CabalRanged(
	w Window,
	sharesAtT0 money.SharesUnits,
	snap *Snapshot,
	end money.Micros,
	flows []domain.Flow,
) (Ranged, error) {
	if err := w.valid(); err != nil {
		return Ranged{}, err
	}
	r := Ranged{T0: w.T0, T1: w.T1, End: end, Flows: flows}
	if sharesAtT0.IsZero() {
		return r, nil
	}
	if !w.usable(snap, sharesAtT0) {
		return Ranged{}, w.unusable("CabalRanged")
	}
	r.Start = snap.Value
	return r, nil
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
