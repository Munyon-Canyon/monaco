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
		"a loss rounds toward zero": {
			first: pnlPoint(3*usd, 0), last: pnlPoint(2*usd, -1*usd),
			pnl: -1 * usd, bps: -3333,
		},
		"a zero base": {
			first: pnlPoint(0, 0), last: pnlPoint(5*usd, 5*usd),
			pnl: 5 * usd, bps: 0,
		},
		"a negative base": {
			first: pnlPoint(1*usd, 0), last: pnlPoint(0, 5*usd),
			pnl: 5 * usd, bps: 0,
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

func TestRangeReturnOfNeedsTwoPoints(t *testing.T) {
	t.Parallel()
	for _, points := range [][]domain.PnLPoint{nil, {pnlPoint(5, 5)}} {
		pnl, bps, err := domain.RangeReturnOf(points)
		if err != nil || !pnl.IsZero() || bps != 0 {
			t.Fatalf("RangeReturnOf(%d points) = %v, %d, %v, want zeros", len(points), pnl, bps, err)
		}
	}
	pnl, bps, err := domain.RangeReturnOf([]domain.PnLPoint{pnlPoint(100, 0), pnlPoint(110, 10), pnlPoint(120, 20)})
	if err != nil || pnl.Int64() != 20 || bps != 2000 {
		t.Fatalf("RangeReturnOf = %v, %d, %v, want 20, 2000", pnl, bps, err)
	}
}

func TestRangeReturnRefusesAFigureThatOverflows(t *testing.T) {
	t.Parallel()
	const bound = math.MaxInt64
	tests := map[string]struct{ first, last domain.PnLPoint }{
		"pnl difference": {first: pnlPoint(0, -bound), last: pnlPoint(bound, bound)},
		"return":         {first: pnlPoint(1, 0), last: pnlPoint(bound+1, bound)},
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
