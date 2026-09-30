package domain_test

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func signed(v int64) money.SignedMicros { return money.SignedMicrosFromInt64(v) }

func bps(v domain.Bps) *domain.Bps { return &v }

func TestLifetime(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		equity  money.Micros
		net     money.SignedMicros
		wantPnL money.SignedMicros
		wantRet *domain.Bps
	}{
		"gain":                    {equity: usd(150), net: signed(100), wantPnL: signed(50), wantRet: bps(5_000)},
		"loss":                    {equity: usd(40), net: signed(100), wantPnL: signed(-60), wantRet: bps(-6_000)},
		"flat":                    {equity: usd(100), net: signed(100), wantPnL: signed(0), wantRet: bps(0)},
		"gain rounds down":        {equity: usd(4), net: signed(3), wantPnL: signed(1), wantRet: bps(3_333)},
		"loss rounds away":        {equity: usd(2), net: signed(3), wantPnL: signed(-1), wantRet: bps(-3_334)},
		"nothing in has no ret":   {equity: usd(5), net: signed(0), wantPnL: signed(5)},
		"cashed out has no ret":   {equity: usd(5), net: signed(-10), wantPnL: signed(15)},
		"int64 edge still prices": {equity: usd(math.MaxInt64), net: signed(0), wantPnL: signed(math.MaxInt64)},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pnl, ret, err := domain.Lifetime(tt.equity, tt.net)
			if err != nil || pnl != tt.wantPnL || !sameBps(ret, tt.wantRet) {
				t.Fatalf("Lifetime = %v, %v, %v, want %v, %v", pnl, fmtBps(ret), err, tt.wantPnL, fmtBps(tt.wantRet))
			}
		})
	}
}

func TestLifetime_Overflow(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		equity money.Micros
		net    money.SignedMicros
	}{
		"pnl past int64":    {equity: usd(math.MaxUint64), net: signed(1)},
		"return past int64": {equity: usd(math.MaxInt64), net: signed(1)},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pnl, ret, err := domain.Lifetime(tt.equity, tt.net)
			if errs.CodeOf(err) != errs.CodeInvalidInput || !pnl.IsZero() || ret != nil {
				t.Fatalf("Lifetime = %v, %v, %v, want invalid_input", pnl, fmtBps(ret), err)
			}
		})
	}
}

func sameBps(a, b *domain.Bps) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func fmtBps(b *domain.Bps) any {
	if b == nil {
		return "nil"
	}
	return *b
}
