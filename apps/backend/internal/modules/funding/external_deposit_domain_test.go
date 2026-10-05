package funding_test

import (
	"math/big"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestClassify_OrderOwnThenUnknownThenDust(t *testing.T) {
	t.Parallel()
	dust, dollar := money.MicrosFromUint64(999_999), money.MicrosFromUint64(1_000_000)
	cases := []struct {
		name string
		in   domain.Inbound
		want domain.Verdict
	}{
		{"own beats everything", domain.Inbound{Owned: true, Value: &dust}, domain.VerdictOwn},
		{"unknown beats dust", domain.Inbound{Value: &dust}, domain.VerdictUnknownAsset},
		{"under a dollar is dust", domain.Inbound{Known: true, Value: &dust}, domain.VerdictDust},
		{"a dollar is detected", domain.Inbound{Known: true, Value: &dollar}, domain.VerdictDetected},
		{"no price is detected", domain.Inbound{Known: true}, domain.VerdictDetected},
	}
	for _, tc := range cases {
		if got := domain.Classify(tc.in); got != tc.want {
			t.Errorf("%s: Classify = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestStockValue_RoundsDownAndNeverOvervalues(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		decimals := rapid.Uint8Range(0, 12).Draw(t, "decimals")
		units := rapid.Uint64Range(0, 1<<50).Draw(t, "units")
		price := rapid.Uint64Range(0, 1<<40).Draw(t, "price")
		num := rapid.Uint64Range(1, 100).Draw(t, "num")
		den := rapid.Uint64Range(1, 100).Draw(t, "den")
		got, err := domain.StockValue(money.NewBaseUnits(units, decimals), money.MicrosFromUint64(price), num, den)
		exact := new(big.Int).Mul(new(big.Int).SetUint64(units), new(big.Int).SetUint64(price))
		exact.Mul(exact, new(big.Int).SetUint64(num))
		scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
		scale.Mul(scale, new(big.Int).SetUint64(den))
		want := new(big.Int).Quo(exact, scale)
		if !want.IsUint64() {
			if err == nil {
				t.Fatalf("StockValue = %s, want an overflow error for %s", got, want)
			}
			return
		}
		if err != nil || got.Uint64() != want.Uint64() {
			t.Fatalf("StockValue = %s, %v, want %s", got, err, want)
		}
	})
}

func TestStockValue_RefusesAZeroMultiplier(t *testing.T) {
	t.Parallel()
	if _, err := domain.StockValue(money.NewBaseUnits(1, 0), money.MicrosFromUint64(1), 0, 1); err == nil {
		t.Fatal("StockValue with a zero multiplier = nil error")
	}
}

func TestStockValue_RefusesAValueOverUint64(t *testing.T) {
	t.Parallel()
	maxUnits := money.NewBaseUnits(^uint64(0), 0)
	if _, err := domain.StockValue(maxUnits, money.MicrosFromUint64(^uint64(0)), 1, 1); err == nil {
		t.Fatal("StockValue past uint64 = nil error")
	}
}

func TestVerdict_StatusAndCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		v      domain.Verdict
		status domain.ExternalDepositStatus
		code   errs.Code
	}{
		{domain.VerdictOwn, "", ""},
		{domain.VerdictUnknownAsset, domain.ExternalIgnoredUnknown, errs.CodeUnknownAsset},
		{domain.VerdictDust, domain.ExternalIgnoredDust, errs.CodeDust},
		{domain.VerdictDetected, domain.ExternalDetected, ""},
	}
	for _, tc := range cases {
		if tc.v.Status() != tc.status || tc.v.Code() != tc.code {
			t.Errorf("verdict %d = %s/%s, want %s/%s", tc.v, tc.v.Status(), tc.v.Code(), tc.status, tc.code)
		}
	}
}
