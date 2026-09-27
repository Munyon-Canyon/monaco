package money_test

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func wantCode(t *testing.T, err error, code errs.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want code %q", code)
	}
	if got := errs.CodeOf(err); got != code {
		t.Fatalf("code = %q, want %q (err %v)", got, code, err)
	}
}

func TestParseMicros(t *testing.T) {
	t.Parallel()
	valid := map[string]uint64{
		"0":                    0,
		"1":                    1,
		"1500000":              1_500_000,
		"18446744073709551615": math.MaxUint64,
	}
	for raw, want := range valid {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			got, err := money.ParseMicros(raw)
			if err != nil {
				t.Fatalf("ParseMicros(%q) err = %v", raw, err)
			}
			if got.Uint64() != want || got.String() != raw {
				t.Fatalf("ParseMicros(%q) = %d %q, want %d", raw, got.Uint64(), got.String(), want)
			}
		})
	}
	invalid := []string{
		"", "-1", "+1", "01", "00", "1.5", "1e6", "1_000", " 1", "1 ", "0x10",
		"18446744073709551616", "99999999999999999999999",
	}
	for _, raw := range invalid {
		t.Run("invalid "+raw, func(t *testing.T) {
			t.Parallel()
			got, err := money.ParseMicros(raw)
			wantCode(t, err, errs.CodeInvalidInput)
			if !got.IsZero() {
				t.Fatalf("ParseMicros(%q) = %v on error, want zero", raw, got)
			}
		})
	}
}

func TestParseSignedMicros(t *testing.T) {
	t.Parallel()
	valid := map[string]int64{
		"0":                    0,
		"-1":                   -1,
		"1500000":              1_500_000,
		"-1500000":             -1_500_000,
		"9223372036854775807":  math.MaxInt64,
		"-9223372036854775808": math.MinInt64,
	}
	for raw, want := range valid {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			got, err := money.ParseSignedMicros(raw)
			if err != nil {
				t.Fatalf("ParseSignedMicros(%q) err = %v", raw, err)
			}
			if got.Int64() != want || got.String() != raw {
				t.Fatalf("ParseSignedMicros(%q) = %d %q, want %d", raw, got.Int64(), got.String(), want)
			}
		})
	}
	invalid := []string{
		"", "-", "-0", "+1", "--1", "-+1", "01", "-01", "1.5", "9223372036854775808", "-9223372036854775809",
	}
	for _, raw := range invalid {
		t.Run("invalid "+raw, func(t *testing.T) {
			t.Parallel()
			_, err := money.ParseSignedMicros(raw)
			wantCode(t, err, errs.CodeInvalidInput)
		})
	}
}

func TestMicrosArithmetic(t *testing.T) {
	t.Parallel()
	a, b := money.MicrosFromUint64(7), money.MicrosFromUint64(5)
	sum, err := a.Add(b)
	if err != nil || sum.Uint64() != 12 {
		t.Fatalf("7+5 = %v, %v; want 12", sum, err)
	}
	diff, err := a.Sub(b)
	if err != nil || diff.Uint64() != 2 {
		t.Fatalf("7-5 = %v, %v; want 2", diff, err)
	}
	zero, err := a.Sub(a)
	if err != nil || !zero.IsZero() {
		t.Fatalf("7-7 = %v, %v; want zero", zero, err)
	}
	if a.IsZero() {
		t.Fatal("7 reports IsZero")
	}
	_, err = b.Sub(a)
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = money.MicrosFromUint64(math.MaxUint64).Add(money.MicrosFromUint64(1))
	wantCode(t, err, errs.CodeInvalidInput)
}

func TestCmp(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b int64
		want int
	}{
		{7, 5, 1},
		{5, 7, -1},
		{7, 7, 0},
		{-3, 3, -1},
	}
	for _, c := range cases {
		if got := money.SignedMicrosFromInt64(c.a).Cmp(money.SignedMicrosFromInt64(c.b)); got != c.want {
			t.Fatalf("SignedMicros Cmp(%d,%d) = %d, want %d", c.a, c.b, got, c.want)
		}
		if c.a < 0 || c.b < 0 {
			continue
		}
		ua, ub := money.MicrosFromUint64(uint64(c.a)), money.MicrosFromUint64(uint64(c.b))
		if got := ua.Cmp(ub); got != c.want {
			t.Fatalf("Micros Cmp(%d,%d) = %d, want %d", c.a, c.b, got, c.want)
		}
		got, err := money.NewBaseUnits(uint64(c.a), 8).Cmp(money.NewBaseUnits(uint64(c.b), 8))
		if err != nil || got != c.want {
			t.Fatalf("BaseUnits Cmp(%d,%d) = %d, %v; want %d", c.a, c.b, got, err, c.want)
		}
	}
}

func TestMicrosDelta(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b uint64
		want int64
	}{
		{7, 5, 2},
		{5, 7, -2},
		{9, 9, 0},
		{math.MaxInt64, 0, math.MaxInt64},
		{0, 1 << 63, math.MinInt64},
	}
	for _, c := range cases {
		d, err := money.MicrosFromUint64(c.a).Delta(money.MicrosFromUint64(c.b))
		if err != nil || d.Int64() != c.want {
			t.Fatalf("Delta(%d,%d) = %v, %v; want %d", c.a, c.b, d, err, c.want)
		}
	}
	overflow := [][2]uint64{{1 << 63, 0}, {0, 1<<63 + 1}, {math.MaxUint64, 0}}
	for _, c := range overflow {
		_, err := money.MicrosFromUint64(c[0]).Delta(money.MicrosFromUint64(c[1]))
		wantCode(t, err, errs.CodeInvalidInput)
	}
}

func TestSignedMicros(t *testing.T) {
	t.Parallel()
	pos, neg := money.SignedMicrosFromInt64(3), money.SignedMicrosFromInt64(-3)
	m, err := pos.Micros()
	if err != nil || m.Uint64() != 3 {
		t.Fatalf("SignedMicros(3).Micros() = %v, %v; want 3", m, err)
	}
	top, err := money.SignedMicrosFromInt64(math.MaxInt64).Micros()
	if err != nil || top.Uint64() != math.MaxInt64 {
		t.Fatalf("SignedMicros(MaxInt64).Micros() = %v, %v", top, err)
	}
	_, err = neg.Micros()
	wantCode(t, err, errs.CodeInvalidInput)
	if pos.IsZero() || !money.SignedMicrosFromInt64(0).IsZero() {
		t.Fatal("IsZero wrong for 3 or 0")
	}
}

func TestBaseUnits(t *testing.T) {
	t.Parallel()
	a, b := money.NewBaseUnits(900, 8), money.NewBaseUnits(100, 8)
	sum, err := a.Add(b)
	if err != nil || sum.Uint64() != 1000 || sum.Decimals() != 8 || sum.String() != "1000" {
		t.Fatalf("900+100 = %v (decimals %d), %v", sum, sum.Decimals(), err)
	}
	diff, err := a.Sub(b)
	if err != nil || diff.Uint64() != 800 || diff.Decimals() != 8 {
		t.Fatalf("900-100 = %v, %v", diff, err)
	}
	zero, err := a.Sub(a)
	if err != nil || !zero.IsZero() || a.IsZero() {
		t.Fatalf("900-900 = %v, %v; want zero", zero, err)
	}
	_, err = b.Sub(a)
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = money.NewBaseUnits(math.MaxUint64, 8).Add(b)
	wantCode(t, err, errs.CodeInvalidInput)
}

func TestBaseUnitsRejectsMismatchedDecimals(t *testing.T) {
	t.Parallel()
	a, other := money.NewBaseUnits(900, 8), money.NewBaseUnits(100, 6)
	_, err := a.Add(other)
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = a.Sub(other)
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = a.Cmp(other)
	wantCode(t, err, errs.CodeInvalidInput)
}

func TestMulDiv(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b, c, want uint64
	}{
		{10, 10, 3, 33},
		{2, 3, 6, 1},
		{0, 5, 7, 0},
		{math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64},
		{math.MaxUint64, 3, 4, 13835058055282163711},
		{1_000_000, 1, 3, 333_333},
	}
	for _, c := range cases {
		got, err := money.MulDiv(c.a, c.b, c.c)
		if err != nil || got != c.want {
			t.Fatalf("MulDiv(%d,%d,%d) = %d, %v; want %d", c.a, c.b, c.c, got, err, c.want)
		}
	}
	_, err := money.MulDiv(1, 1, 0)
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = money.MulDiv(math.MaxUint64, 2, 1)
	wantCode(t, err, errs.CodeInvalidInput)
}
