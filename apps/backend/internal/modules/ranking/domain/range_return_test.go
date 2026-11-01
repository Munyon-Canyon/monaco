package domain_test

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func pnlPoint(equity uint64, pnl int64) domain.PnLPoint {
	return domain.PnLPoint{Equity: money.MicrosFromUint64(equity), PnL: money.SignedMicrosFromInt64(pnl)}
}

func bp(n domain.Bps) *domain.Bps { return &n }

func showBps(b *domain.Bps) string {
	if b == nil {
		return "nil"
	}
	return strconv.FormatInt(int64(*b), 10)
}

func TestRangeReturn(t *testing.T) {
	t.Parallel()
	const usd = 1_000_000
	tests := map[string]struct {
		first, last domain.PnLPoint
		pnl         int64
		bps         *domain.Bps
	}{
		"the fixture month": {
			first: pnlPoint(11_841*usd, 1_000*usd), last: pnlPoint(12_082_100_000, 1_241_100_000),
			pnl: 241_100_000, bps: bp(203),
		},
		"a deposit mid-range is not a gain": {
			first: pnlPoint(10_000*usd, 0), last: pnlPoint(11_000*usd, 0),
			pnl: 0, bps: bp(0),
		},
		"a deposit and a gain": {
			first: pnlPoint(10_000*usd, 0), last: pnlPoint(11_500*usd, 500*usd),
			pnl: 500 * usd, bps: bp(454),
		},
		"a loss": {
			first: pnlPoint(10_000*usd, 0), last: pnlPoint(9_500*usd, -500*usd),
			pnl: -500 * usd, bps: bp(-500),
		},
		"a loss rounds down to the next basis point": {
			first: pnlPoint(3*usd, 0), last: pnlPoint(2*usd, -1*usd),
			pnl: -1 * usd, bps: bp(-3334),
		},
		"a zero base": {
			first: pnlPoint(0, 0), last: pnlPoint(5*usd, 5*usd),
			pnl: 5 * usd, bps: nil,
		},
		"a cash-out past the start value": {
			first: pnlPoint(1*usd, 0), last: pnlPoint(0, 5*usd),
			pnl: 5 * usd, bps: bp(50000),
		},
		"a base under the floor": {
			first: pnlPoint(100, 0), last: pnlPoint(200, 100),
			pnl: 100, bps: nil,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pnl, bps, err := domain.RangeReturn(tt.first, tt.last)
			if err != nil {
				t.Fatal(err)
			}
			if pnl.Int64() != tt.pnl || !sameBps(bps, tt.bps) {
				t.Fatalf("RangeReturn = %d, %s, want %d, %s", pnl.Int64(), showBps(bps), tt.pnl, showBps(tt.bps))
			}
		})
	}
}

func TestRangeReturnOfIgnoresCashOuts(t *testing.T) {
	t.Parallel()
	const usd = 1_000_000
	tests := map[string]struct {
		points []domain.PnLPoint
		pnl    int64
		bps    *domain.Bps
	}{
		"exiting one of two cabals after a 1% loss": {
			points: []domain.PnLPoint{pnlPoint(1_100*usd, 0), pnlPoint(100*usd, -10*usd)},
			pnl:    -10 * usd, bps: bp(-91),
		},
		"exiting the only cabal after a 1% loss": {
			points: []domain.PnLPoint{pnlPoint(1_000*usd, 0), pnlPoint(0, -10*usd)},
			pnl:    -10 * usd, bps: bp(-100),
		},
		"exiting the only cabal after a gain": {
			points: []domain.PnLPoint{pnlPoint(1_000*usd, 0), pnlPoint(0, 10*usd)},
			pnl:    10 * usd, bps: bp(100),
		},
		"a deposit then a cash-out counts the deposit once": {
			points: []domain.PnLPoint{pnlPoint(1_000*usd, 0), pnlPoint(2_000*usd, 0), pnlPoint(1_010*usd, 10*usd)},
			pnl:    10 * usd, bps: bp(50),
		},
		"a base under the floor shows no percent": {
			points: []domain.PnLPoint{pnlPoint(usd/200, 0), pnlPoint(usd/100, usd/200)},
			pnl:    usd / 200, bps: nil,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pnl, bps, err := domain.RangeReturnOf(tt.points)
			if err != nil {
				t.Fatal(err)
			}
			if pnl.Int64() != tt.pnl || !sameBps(bps, tt.bps) {
				t.Fatalf("RangeReturnOf = %d, %s, want %d, %s", pnl.Int64(), showBps(bps), tt.pnl, showBps(tt.bps))
			}
		})
	}
}

func TestRangeReturnOfNeedsTwoPoints(t *testing.T) {
	t.Parallel()
	for _, points := range [][]domain.PnLPoint{nil, {pnlPoint(5, 5)}} {
		pnl, bps, err := domain.RangeReturnOf(points)
		if err != nil || !pnl.IsZero() || bps != nil {
			t.Fatalf("RangeReturnOf(%d points) = %v, %v, %v, want zero and no percent", len(points), pnl, bps, err)
		}
	}
	pnl, bps, err := domain.RangeReturnOf(
		[]domain.PnLPoint{
			pnlPoint(100_000_000, 0),
			pnlPoint(110_000_000, 10_000_000),
			pnlPoint(120_000_000, 20_000_000),
		},
	)
	if err != nil || pnl.Int64() != 20_000_000 || !sameBps(bps, bp(2000)) {
		t.Fatalf("RangeReturnOf = %v, %s, %v, want 20000000, 2000", pnl, showBps(bps), err)
	}
}

func TestRangeReturnRefusesAFigureThatOverflows(t *testing.T) {
	t.Parallel()
	const bound = math.MaxInt64
	tests := map[string]struct{ first, last domain.PnLPoint }{
		"pnl difference": {first: pnlPoint(0, -bound), last: pnlPoint(bound, bound)},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := domain.RangeReturn(tt.first, tt.last); errs.CodeOf(err) != errs.CodeInvalidInput {
				t.Fatalf("err = %v, want invalid_input", err)
			}
		})
	}
}

func TestRangeGainIsTheHomeFormula(t *testing.T) {
	t.Parallel()
	const usd = 1_000_000
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(30 * 24 * time.Hour)
	at := func(d time.Duration) domain.Flow { return domain.Flow{At: t0.Add(d)} }
	deposit := func(d time.Duration, amount int64) domain.Flow {
		f := at(d)
		f.Amount = money.SignedMicrosFromInt64(amount)
		return f
	}
	tests := map[string]struct {
		in  domain.DietzInput
		pnl int64
		bps *domain.Bps
	}{
		"a mid-range deposit": {
			in: domain.DietzInput{
				T0: t0, T1: t1, Start: money.MicrosFromUint64(1_000 * usd), End: money.MicrosFromUint64(2_200 * usd),
				Flows: []domain.Flow{deposit(15*24*time.Hour, 1_000*usd)},
			},
			pnl: 200 * usd, bps: bp(1000),
		},
		"a cash-out is not taken off the base": {
			in: domain.DietzInput{
				T0: t0, T1: t1, Start: money.MicrosFromUint64(1_000 * usd), End: money.MicrosFromUint64(0),
				Flows: []domain.Flow{deposit(15*24*time.Hour, -1_010*usd)},
			},
			pnl: 10 * usd, bps: bp(100),
		},
		"a fresh fund a minute before the end cannot explode": {
			in: domain.DietzInput{
				T0: t0, T1: t1, End: money.MicrosFromUint64(100_100_000),
				Flows: []domain.Flow{deposit(30*24*time.Hour-time.Minute, 100*usd)},
			},
			pnl: 100_000, bps: bp(10),
		},
		"no base shows no percent": {
			in:  domain.DietzInput{T0: t0, T1: t1, End: money.MicrosFromUint64(5 * usd)},
			pnl: 5 * usd, bps: nil,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pnl, bps, err := domain.RangeGain(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if pnl.Int64() != tt.pnl || !sameBps(bps, tt.bps) {
				t.Fatalf("RangeGain = %d, %s, want %d, %s", pnl.Int64(), showBps(bps), tt.pnl, showBps(tt.bps))
			}
		})
	}
}
