package domain_test

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func pnlPoint(equity uint64, pnl int64) domain.PnLPoint {
	return domain.PnLPoint{Equity: money.MicrosFromUint64(equity), PnL: money.SignedMicrosFromInt64(pnl)}
}

func TestRangeReturn(t *testing.T) {
	t.Parallel()
	const usd = 1_000_000
	tests := map[string]struct {
		first, last domain.PnLPoint
		pnl         int64
		bps         domain.Bps
	}{
		"the fixture month": {
			first: pnlPoint(11_841*usd, 1_000*usd), last: pnlPoint(12_082_100_000, 1_241_100_000),
			pnl: 241_100_000, bps: 203,
		},
		"a deposit mid-range is not a gain": {
			first: pnlPoint(10_000*usd, 0), last: pnlPoint(11_000*usd, 0),
			pnl: 0, bps: 0,
		},
		"a deposit and a gain": {
			first: pnlPoint(10_000*usd, 0), last: pnlPoint(11_500*usd, 500*usd),
			pnl: 500 * usd, bps: 454,
		},
		"a loss": {
			first: pnlPoint(10_000*usd, 0), last: pnlPoint(9_500*usd, -500*usd),
			pnl: -500 * usd, bps: -500,
		},
		"a loss rounds down to the next basis point": {
			first: pnlPoint(3*usd, 0), last: pnlPoint(2*usd, -1*usd),
			pnl: -1 * usd, bps: -3334,
		},
		"a zero base": {
			first: pnlPoint(0, 0), last: pnlPoint(5*usd, 5*usd),
			pnl: 5 * usd, bps: 0,
		},
		"a cash-out past the start value": {
			first: pnlPoint(1*usd, 0), last: pnlPoint(0, 5*usd),
			pnl: 5 * usd, bps: 50000,
		},
		"a base under the floor": {
			first: pnlPoint(100, 0), last: pnlPoint(200, 100),
			pnl: 100, bps: 0,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pnl, bps, err := domain.RangeReturn(tt.first, tt.last)
			if err != nil {
				t.Fatal(err)
			}
			if pnl.Int64() != tt.pnl || bps != tt.bps {
				t.Fatalf("RangeReturn = %d, %d, want %d, %d", pnl.Int64(), bps, tt.pnl, tt.bps)
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
		bps    domain.Bps
	}{
		"exiting one of two cabals after a 1% loss": {
			points: []domain.PnLPoint{pnlPoint(1_100*usd, 0), pnlPoint(100*usd, -10*usd)},
			pnl:    -10 * usd, bps: -91,
		},
		"exiting the only cabal after a 1% loss": {
			points: []domain.PnLPoint{pnlPoint(1_000*usd, 0), pnlPoint(0, -10*usd)},
			pnl:    -10 * usd, bps: -100,
		},
		"exiting the only cabal after a gain": {
			points: []domain.PnLPoint{pnlPoint(1_000*usd, 0), pnlPoint(0, 10*usd)},
			pnl:    10 * usd, bps: 100,
		},
		"a deposit then a cash-out counts the deposit once": {
			points: []domain.PnLPoint{pnlPoint(1_000*usd, 0), pnlPoint(2_000*usd, 0), pnlPoint(1_010*usd, 10*usd)},
			pnl:    10 * usd, bps: 50,
		},
		"a base under the floor shows no percent": {
			points: []domain.PnLPoint{pnlPoint(usd/200, 0), pnlPoint(usd/100, usd/200)},
			pnl:    usd / 200, bps: 0,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pnl, bps, err := domain.RangeReturnOf(tt.points)
			if err != nil {
				t.Fatal(err)
			}
			if pnl.Int64() != tt.pnl || bps != tt.bps {
				t.Fatalf("RangeReturnOf = %d, %d, want %d, %d", pnl.Int64(), bps, tt.pnl, tt.bps)
			}
		})
	}
}

func TestRangeReturnOfNeedsTwoPoints(t *testing.T) {
	t.Parallel()
	for _, points := range [][]domain.PnLPoint{nil, {pnlPoint(5, 5)}} {
		pnl, bps, err := domain.RangeReturnOf(points)
		if err != nil || !pnl.IsZero() || bps != 0 {
			t.Fatalf("RangeReturnOf(%d points) = %v, %d, %v, want zeros", len(points), pnl, bps, err)
		}
	}
	pnl, bps, err := domain.RangeReturnOf(
		[]domain.PnLPoint{
			pnlPoint(100_000_000, 0),
			pnlPoint(110_000_000, 10_000_000),
			pnlPoint(120_000_000, 20_000_000),
		},
	)
	if err != nil || pnl.Int64() != 20_000_000 || bps != 2000 {
		t.Fatalf("RangeReturnOf = %v, %d, %v, want 20000000, 2000", pnl, bps, err)
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
