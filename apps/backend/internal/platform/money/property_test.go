package money_test

import (
	"math"
	"math/big"
	"strconv"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func anyUint64(t *rapid.T, label string) uint64 {
	return rapid.OneOf(
		rapid.Uint64(),
		rapid.Uint64Range(0, 1<<32),
		rapid.Uint64Range(math.MaxUint64-(1<<16), math.MaxUint64),
	).Draw(t, label)
}

func TestMulDivRoundsDownWithinOneUnit(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		a, b := anyUint64(t, "a"), anyUint64(t, "b")
		c := rapid.OneOf(rapid.Uint64Range(1, math.MaxUint64), rapid.Uint64Range(1, 1<<20)).Draw(t, "c")
		product := new(big.Int).Mul(new(big.Int).SetUint64(a), new(big.Int).SetUint64(b))
		exactFits := new(big.Int).Quo(product, new(big.Int).SetUint64(c)).IsUint64()
		got, err := money.MulDiv(a, b, c)
		if !exactFits {
			if err == nil {
				t.Fatalf("MulDiv(%d,%d,%d) = %d, want overflow error", a, b, c, got)
			}
			return
		}
		if err != nil {
			t.Fatalf("MulDiv(%d,%d,%d) err = %v", a, b, c, err)
		}
		cBig := new(big.Int).SetUint64(c)
		low := new(big.Int).Mul(new(big.Int).SetUint64(got), cBig)
		high := new(big.Int).Add(low, cBig)
		if low.Cmp(product) > 0 {
			t.Fatalf("MulDiv(%d,%d,%d) = %d exceeds a*b/c", a, b, c, got)
		}
		if high.Cmp(product) <= 0 {
			t.Fatalf("MulDiv(%d,%d,%d) = %d is 1 or more units below a*b/c", a, b, c, got)
		}
	})
}

func TestMicrosAddSubRoundTrip(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		a := money.MicrosFromUint64(anyUint64(t, "a"))
		b := money.MicrosFromUint64(anyUint64(t, "b"))
		sum, err := a.Add(b)
		if a.Uint64() > math.MaxUint64-b.Uint64() {
			if err == nil {
				t.Fatalf("%v + %v = %v, want overflow error", a, b, sum)
			}
			return
		}
		if err != nil {
			t.Fatalf("%v + %v err = %v", a, b, err)
		}
		back, err := sum.Sub(b)
		if err != nil || back != a {
			t.Fatalf("(%v + %v) - %v = %v, %v; want %v", a, b, b, back, err, a)
		}
	})
}

func TestMicrosDeltaMatchesExactDifference(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		a := money.MicrosFromUint64(anyUint64(t, "a"))
		b := money.MicrosFromUint64(anyUint64(t, "b"))
		exact := new(big.Int).Sub(new(big.Int).SetUint64(a.Uint64()), new(big.Int).SetUint64(b.Uint64()))
		d, err := a.Delta(b)
		if !exact.IsInt64() {
			if err == nil {
				t.Fatalf("Delta(%v,%v) = %v, want overflow error", a, b, d)
			}
			return
		}
		if err != nil || d.Int64() != exact.Int64() {
			t.Fatalf("Delta(%v,%v) = %v, %v; want %s", a, b, d, err, exact)
		}
		if a.Cmp(b) >= 0 {
			checkDeltaConvertsBack(t, a, b, d)
		}
	})
}

func checkDeltaConvertsBack(t *rapid.T, a, b money.Micros, d money.SignedMicros) {
	back, err := d.Micros()
	if err != nil {
		t.Fatalf("Delta(%v,%v).Micros() err = %v", a, b, err)
	}
	if sum, err := b.Add(back); err != nil || sum != a {
		t.Fatalf("%v + Delta(%v,%v) = %v, %v; want %v", b, a, b, sum, err, a)
	}
}

func TestMicrosStringParseRoundTrip(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		m := money.MicrosFromUint64(anyUint64(t, "v"))
		back, err := money.ParseMicros(m.String())
		if err != nil || back != m {
			t.Fatalf("ParseMicros(%q) = %v, %v", m.String(), back, err)
		}
		s := money.SignedMicrosFromInt64(rapid.Int64().Draw(t, "s"))
		sBack, err := money.ParseSignedMicros(s.String())
		if err != nil || sBack != s {
			t.Fatalf("ParseSignedMicros(%q) = %v, %v", s.String(), sBack, err)
		}
	})
}

func FuzzParseMicros(f *testing.F) {
	for _, seed := range []string{"0", "1500000", "18446744073709551615", "18446744073709551616", "01", "-1", "+1", ""} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		m, err := money.ParseMicros(raw)
		want, stdErr := strconv.ParseUint(raw, 10, 64)
		canonical := stdErr == nil && strconv.FormatUint(want, 10) == raw
		if canonical != (err == nil) {
			t.Fatalf("ParseMicros(%q) err = %v, canonical = %v", raw, err, canonical)
		}
		if err == nil && (m.Uint64() != want || m.String() != raw) {
			t.Fatalf("ParseMicros(%q) = %d %q, want %d", raw, m.Uint64(), m.String(), want)
		}
	})
}
