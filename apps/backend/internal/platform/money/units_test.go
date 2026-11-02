package money_test

import (
	"context"
	"encoding/json"
	"math"
	"math/big"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func pow10(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }

func maxUnits() *big.Int {
	return new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
}

func anyUnitsBig(t *rapid.T, label string) *big.Int {
	return rapid.OneOf(
		rapid.Custom(func(t *rapid.T) *big.Int { return new(big.Int).SetUint64(anyUint64(t, "small")) }),
		rapid.Custom(func(t *rapid.T) *big.Int {
			buf := rapid.SliceOfN(rapid.Byte(), 0, 32).Draw(t, "bytes")
			return new(big.Int).SetBytes(buf)
		}),
		rapid.Custom(func(t *rapid.T) *big.Int {
			return new(big.Int).Mul(new(big.Int).SetUint64(anyUint64(t, "whole")), pow10(18))
		}),
	).Draw(t, label)
}

func mustUnits(tb testing.TB, raw string) money.Units {
	tb.Helper()
	u, err := money.ParseUnits(raw)
	if err != nil {
		tb.Fatalf("ParseUnits(%q) = %v", raw, err)
	}
	return u
}

func mustSigned(tb testing.TB, raw string) money.SignedUnits {
	tb.Helper()
	s, err := money.ParseSignedUnits(raw)
	if err != nil {
		tb.Fatalf("ParseSignedUnits(%q) = %v", raw, err)
	}
	return s
}

func TestUnitsZeroValueIsZero(t *testing.T) {
	t.Parallel()
	var u money.Units
	var s money.SignedUnits
	if !u.IsZero() || u.String() != "0" || !s.IsZero() || s.String() != "0" || s.Sign() != 0 {
		t.Fatalf("zero values = %q %q", u, s)
	}
	if !u.Equal(money.UnitsFromUint64(0)) || !s.Equal(money.SignedUnitsFromInt64(0)) {
		t.Fatal("zero value is not equal to an explicit zero")
	}
	sum, err := u.Add(u)
	if err != nil || !sum.IsZero() {
		t.Fatalf("0 + 0 = %v, %v", sum, err)
	}
	if got := u.Big(); got.Sign() != 0 {
		t.Fatalf("Big() = %v", got)
	}
	if got := s.Big(); got.Sign() != 0 {
		t.Fatalf("SignedUnits.Big() = %v", got)
	}
}

func TestUnitsHoldPastUint64(t *testing.T) {
	t.Parallel()
	tenTokens := mustUnits(t, "10000000000000000000")
	twentyFive := mustUnits(t, "25000000000000000000")
	if !tenTokens.IsUint64() || twentyFive.IsUint64() {
		t.Fatalf("10 tokens fit uint64 = %v, 25 tokens fit uint64 = %v", tenTokens.IsUint64(), twentyFive.IsUint64())
	}
	sum, err := tenTokens.Add(twentyFive)
	if err != nil || sum.String() != "35000000000000000000" {
		t.Fatalf("10 + 25 = %v, %v", sum, err)
	}
	back, err := sum.Sub(twentyFive)
	if err != nil || !back.Equal(tenTokens) {
		t.Fatalf("(10 + 25) - 25 = %v, %v", back, err)
	}
	if _, err := twentyFive.Uint64(); !codeIs(err, errs.CodeInvalidInput) {
		t.Fatalf("Uint64 past 2^64 err = %v", err)
	}
	if _, err := twentyFive.Micros(); !codeIs(err, errs.CodeInvalidInput) {
		t.Fatalf("Micros past 2^64 err = %v", err)
	}
	if _, err := tenTokens.Int64(); !codeIs(err, errs.CodeInvalidInput) {
		t.Fatalf("Int64 past 2^63 err = %v", err)
	}
}

func TestUnitsSmallValuesConvert(t *testing.T) {
	t.Parallel()
	u := money.UnitsFromUint64(math.MaxInt64)
	i, err := u.Int64()
	if err != nil || i != math.MaxInt64 {
		t.Fatalf("Int64 = %d, %v", i, err)
	}
	v, err := u.Uint64()
	if err != nil || v != math.MaxInt64 {
		t.Fatalf("Uint64 = %d, %v", v, err)
	}
	m, err := money.UnitsFromUint64(1_500_000).Micros()
	if err != nil || m.Uint64() != 1_500_000 {
		t.Fatalf("Micros = %v, %v", m, err)
	}
	if got := money.UnitsFromMicros(money.MicrosFromUint64(7)); got.String() != "7" {
		t.Fatalf("UnitsFromMicros = %v", got)
	}
	if got := money.SignedUnitsFromMicros(money.SignedMicrosFromInt64(-7)); got.String() != "-7" {
		t.Fatalf("SignedUnitsFromMicros = %v", got)
	}
}

func TestInt64OrZeroIsZeroPastInt64(t *testing.T) {
	t.Parallel()
	if got := money.UnitsFromUint64(math.MaxInt64).Int64OrZero(); got != math.MaxInt64 {
		t.Fatalf("Int64OrZero at int64 max = %d", got)
	}
	if got := mustUnits(t, "9223372036854775808").Int64OrZero(); got != 0 {
		t.Fatalf("Int64OrZero past int64 = %d", got)
	}
	if got := mustSigned(t, "-9223372036854775808").Int64OrZero(); got != math.MinInt64 {
		t.Fatalf("signed Int64OrZero at int64 min = %d", got)
	}
	if got := mustSigned(t, "-9223372036854775809").Int64OrZero(); got != 0 {
		t.Fatalf("signed Int64OrZero past int64 = %d", got)
	}
}

func TestUnitsStringOrNilIsNilOnlyForZero(t *testing.T) {
	t.Parallel()
	if got := (money.Units{}).StringOrNil(); got != nil {
		t.Fatalf("zero value = %q", *got)
	}
	if got := money.UnitsFromUint64(0).StringOrNil(); got != nil {
		t.Fatalf("zero = %q", *got)
	}
	got := mustUnits(t, "25000000000000000000").StringOrNil()
	if got == nil || *got != "25000000000000000000" {
		t.Fatalf("past int64 = %v", got)
	}
}

func TestPricePerUnitsMatchesNarrowPriceAndHoldsPastUint64(t *testing.T) {
	t.Parallel()
	usdc := money.MicrosFromUint64(25_000_000)
	if got, want := money.PricePerUnits(
		usdc,
		money.UnitsFromUint64(4_000_000),
		6,
	), money.PricePerUnit(
		usdc,
		4_000_000,
		6,
	); got != want {
		t.Fatalf("PricePerUnits = %v, want the narrow %v", got, want)
	}
	if got := money.PricePerUnits(usdc, mustUnits(t, "25000000000000000000"), 18); got.Uint64() != 1_000_000 {
		t.Fatalf("25 tokens for 25 USDC at 18 decimals = %v, want 1000000", got)
	}
	if got := money.PricePerUnits(usdc, money.Units{}, 18); !got.IsZero() {
		t.Fatalf("price of zero tokens = %v", got)
	}
	if got := money.PricePerUnits(usdc, money.UnitsFromUint64(1), 78); !got.IsZero() {
		t.Fatalf("price at 78 decimals = %v, want zero for an unrepresentable scale", got)
	}
	if got := money.PricePerUnits(money.MicrosFromUint64(math.MaxUint64), money.UnitsFromUint64(1), 77); !got.IsZero() {
		t.Fatalf("price past 2^256 = %v, want zero", got)
	}
	if got := money.SignedMicrosFromInt64(-5).Big(); got.Int64() != -5 {
		t.Fatalf("SignedMicros.Big = %v", got)
	}
	if got := money.PricePerUnits(money.MicrosFromUint64(math.MaxUint64), money.UnitsFromUint64(1), 18); !got.IsZero() {
		t.Fatalf("price past uint64 = %v, want zero", got)
	}
}

func TestUnitsSubBelowZeroFails(t *testing.T) {
	t.Parallel()
	if _, err := money.UnitsFromUint64(1).Sub(money.UnitsFromUint64(2)); !codeIs(err, errs.CodeInvalidInput) {
		t.Fatalf("1 - 2 err = %v", err)
	}
}

func TestUnitsBoundsAt256Bits(t *testing.T) {
	t.Parallel()
	top := maxUnits()
	over := new(big.Int).Add(top, big.NewInt(1))
	u, err := money.UnitsFromBig(top)
	if err != nil || u.Big().Cmp(top) != 0 {
		t.Fatalf("UnitsFromBig(2^256-1) = %v, %v", u, err)
	}
	if _, err := u.Add(money.UnitsFromUint64(1)); !codeIs(err, errs.CodeInvalidInput) {
		t.Fatalf("2^256-1 + 1 err = %v", err)
	}
	for name, v := range map[string]*big.Int{"2^256": over, "-1": big.NewInt(-1), "nil": nil} {
		if _, err := money.UnitsFromBig(v); !codeIs(err, errs.CodeInvalidInput) {
			t.Fatalf("UnitsFromBig(%s) err = %v", name, err)
		}
	}
	if _, err := money.ParseUnits(over.String()); !codeIs(err, errs.CodeInvalidInput) {
		t.Fatalf("ParseUnits(2^256) err = %v", err)
	}
}

func TestSignedUnitsBoundsAt256Bits(t *testing.T) {
	t.Parallel()
	top := maxUnits()
	over := new(big.Int).Add(top, big.NewInt(1))
	s, err := money.SignedUnitsFromBig(new(big.Int).Neg(top))
	if err != nil {
		t.Fatalf("SignedUnitsFromBig(-(2^256-1)) = %v", err)
	}
	if _, err := s.Add(s); !codeIs(err, errs.CodeInvalidInput) {
		t.Fatalf("signed overflow err = %v", err)
	}
	for name, v := range map[string]*big.Int{"-2^256": new(big.Int).Neg(over), "nil": nil} {
		if _, err := money.SignedUnitsFromBig(v); !codeIs(err, errs.CodeInvalidInput) {
			t.Fatalf("SignedUnitsFromBig(%s) err = %v", name, err)
		}
	}
	if _, err := money.ParseSignedUnits("-" + over.String()); !codeIs(err, errs.CodeInvalidInput) {
		t.Fatalf("ParseSignedUnits(-2^256) err = %v", err)
	}
}

func TestUnitsFitInNumeric78(t *testing.T) {
	t.Parallel()
	if got := len(maxUnits().String()); got > 78 {
		t.Fatalf("2^256-1 has %d digits, numeric(78,0) holds 78", got)
	}
}

func TestUnitsParseRejectsNonCanonical(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", "-", "+1", "01", "1.5", "1e3", "0x10", " 1", "1 ", "-1", "1_0", "--1"} {
		if _, err := money.ParseUnits(raw); !codeIs(err, errs.CodeInvalidInput) {
			t.Fatalf("ParseUnits(%q) err = %v", raw, err)
		}
	}
	for _, raw := range []string{"", "-", "+1", "01", "-01", "-0", "1.5", "0x10", "1_0", "--1"} {
		if _, err := money.ParseSignedUnits(raw); !codeIs(err, errs.CodeInvalidInput) {
			t.Fatalf("ParseSignedUnits(%q) err = %v", raw, err)
		}
	}
}

func TestUnitsSignedConversions(t *testing.T) {
	t.Parallel()
	neg := mustSigned(t, "-5")
	if _, err := neg.Units(); !codeIs(err, errs.CodeInvalidInput) {
		t.Fatalf("(-5).Units() err = %v", err)
	}
	if neg.Sign() != -1 || neg.Abs().String() != "5" || neg.Neg().String() != "5" {
		t.Fatalf("neg = %v abs %v neg %v", neg, neg.Abs(), neg.Neg())
	}
	pos, err := mustSigned(t, "5").Units()
	if err != nil || pos.String() != "5" {
		t.Fatalf("5.Units() = %v, %v", pos, err)
	}
	if got := pos.Signed(); got.String() != "5" {
		t.Fatalf("Signed() = %v", got)
	}
	i, err := mustSigned(t, "-9223372036854775808").Int64()
	if err != nil || i != math.MinInt64 {
		t.Fatalf("Int64 = %d, %v", i, err)
	}
	if _, err := mustSigned(t, "9223372036854775808").Int64(); !codeIs(err, errs.CodeInvalidInput) {
		t.Fatalf("Int64 past int64 err = %v", err)
	}
	if mustSigned(t, "3").Cmp(mustSigned(t, "-3")) != 1 {
		t.Fatal("3 should compare above -3")
	}
}

func TestUnitsMulDivRoundsDown(t *testing.T) {
	t.Parallel()
	got, err := money.UnitsMulDiv(
		mustUnits(t, "25000000000000000000"),
		money.UnitsFromUint64(3),
		money.UnitsFromUint64(7),
	)
	if err != nil || got.String() != "10714285714285714285" {
		t.Fatalf("25e18 * 3 / 7 = %v, %v", got, err)
	}
	if _, err := money.UnitsMulDiv(
		money.UnitsFromUint64(1),
		money.UnitsFromUint64(1),
		money.Units{},
	); !codeIs(
		err,
		errs.CodeInvalidInput,
	) {
		t.Fatalf("divide by zero err = %v", err)
	}
	top, _ := money.UnitsFromBig(maxUnits())
	if _, err := money.UnitsMulDiv(
		top,
		money.UnitsFromUint64(2),
		money.UnitsFromUint64(1),
	); !codeIs(
		err,
		errs.CodeInvalidInput,
	) {
		t.Fatalf("overflowing product err = %v", err)
	}
}

func TestUnitsAreImmutable(t *testing.T) {
	t.Parallel()
	src := big.NewInt(100)
	u, err := money.UnitsFromBig(src)
	if err != nil {
		t.Fatal(err)
	}
	src.SetInt64(5)
	u.Big().SetInt64(7)
	if u.String() != "100" {
		t.Fatalf("units changed through an alias: %v", u)
	}
	s, err := money.SignedUnitsFromBig(src)
	if err != nil {
		t.Fatal(err)
	}
	src.SetInt64(9)
	s.Big().SetInt64(11)
	if s.String() != "5" {
		t.Fatalf("signed units changed through an alias: %v", s)
	}
	a, b := money.UnitsFromUint64(1), money.UnitsFromUint64(2)
	if _, err := a.Add(b); err != nil || a.String() != "1" || b.String() != "2" {
		t.Fatalf("Add changed an operand: %v %v", a, b)
	}
}

func drawUnits(t *rapid.T, label string) money.Units {
	u, err := money.UnitsFromBig(anyUnitsBig(t, label))
	if err != nil {
		t.Skip()
	}
	return u
}

func checkUnitsSumRoundTrip(t *rapid.T, a, b, sum money.Units) {
	exact := new(big.Int).Add(a.Big(), b.Big())
	if sum.Big().Cmp(exact) != 0 {
		t.Fatalf("%v + %v = %v; want %v", a, b, sum, exact)
	}
	back, err := sum.Sub(b)
	if err != nil || !back.Equal(a) {
		t.Fatalf("(%v + %v) - %v = %v, %v; want %v", a, b, b, back, err, a)
	}
	if sum.Cmp(a) < 0 || sum.Cmp(b) < 0 {
		t.Fatalf("%v + %v = %v is below an operand", a, b, sum)
	}
}

func TestUnitsAddSubProperty(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		a, b := drawUnits(t, "a"), drawUnits(t, "b")
		sum, err := a.Add(b)
		if new(big.Int).Add(a.Big(), b.Big()).BitLen() > 256 {
			if err == nil {
				t.Fatalf("%v + %v = %v, want overflow error", a, b, sum)
			}
			return
		}
		if err != nil {
			t.Fatalf("%v + %v err = %v", a, b, err)
		}
		checkUnitsSumRoundTrip(t, a, b, sum)
	})
}

func checkUnitsTextRoundTrip(t *rapid.T, u money.Units) {
	back, err := money.ParseUnits(u.String())
	if err != nil || !back.Equal(u) {
		t.Fatalf("ParseUnits(%q) = %v, %v", u.String(), back, err)
	}
	raw, err := json.Marshal(u)
	if err != nil || string(raw) != `"`+u.String()+`"` {
		t.Fatalf("json = %s, %v", raw, err)
	}
	var viaJSON money.Units
	if err := json.Unmarshal(raw, &viaJSON); err != nil || !viaJSON.Equal(u) {
		t.Fatalf("json round trip = %v, %v", viaJSON, err)
	}
}

func checkSignedTextRoundTrip(t *rapid.T, s money.SignedUnits) {
	back, err := money.ParseSignedUnits(s.String())
	if err != nil || !back.Equal(s) {
		t.Fatalf("ParseSignedUnits(%q) = %v, %v", s.String(), back, err)
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var viaJSON money.SignedUnits
	if err := json.Unmarshal(raw, &viaJSON); err != nil || !viaJSON.Equal(s) {
		t.Fatalf("signed json round trip = %v, %v", viaJSON, err)
	}
}

func TestUnitsStringParseJSONRoundTrip(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		u := drawUnits(t, "v")
		checkUnitsTextRoundTrip(t, u)
		s := u.Signed()
		if rapid.Bool().Draw(t, "negate") {
			s = s.Neg()
		}
		checkSignedTextRoundTrip(t, s)
	})
}

func TestUnitsJSONMatchesUint64StringTag(t *testing.T) {
	t.Parallel()
	type legacy struct {
		Amount uint64 `json:"amount,string"`
	}
	type wide struct {
		Amount money.Units `json:"amount"`
	}
	old, err := json.Marshal(legacy{Amount: math.MaxUint64})
	if err != nil {
		t.Fatal(err)
	}
	var w wide
	if err := json.Unmarshal(old, &w); err != nil {
		t.Fatalf("decode legacy wire %s: %v", old, err)
	}
	now, err := json.Marshal(w)
	if err != nil || string(now) != string(old) {
		t.Fatalf("wide json = %s, %v; legacy = %s", now, err, old)
	}
}

func TestUnitsJSONRejectsNonCanonical(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`1`, `"-1"`, `"1.5"`, `"01"`, `""`} {
		var u money.Units
		if err := json.Unmarshal([]byte(raw), &u); err == nil {
			t.Fatalf("Unmarshal(%s) = %v, want error", raw, u)
		}
	}
	for _, raw := range []string{`1`, `"+1"`, `"1.5"`, `"-0"`, `""`} {
		var s money.SignedUnits
		if err := json.Unmarshal([]byte(raw), &s); err == nil {
			t.Fatalf("Unmarshal(%s) = %v, want error", raw, s)
		}
	}
}

func TestUnitsScanAndValue(t *testing.T) {
	t.Parallel()
	var u money.Units
	if err := u.Scan([]byte("25000000000000000000")); err != nil || u.String() != "25000000000000000000" {
		t.Fatalf("Scan bytes = %v, %v", u, err)
	}
	if err := u.Scan("7"); err != nil || u.String() != "7" {
		t.Fatalf("Scan string = %v, %v", u, err)
	}
	for _, src := range []any{int64(7), "-1"} {
		if err := u.Scan(src); !codeIs(err, errs.CodeDecodeFailed) {
			t.Fatalf("Scan %v err = %v", src, err)
		}
	}
	if v, err := u.Value(); err != nil || v != "7" {
		t.Fatalf("Value = %v, %v", v, err)
	}
}

func TestSignedUnitsScanAndValue(t *testing.T) {
	t.Parallel()
	var s money.SignedUnits
	if err := s.Scan("-25000000000000000000"); err != nil || s.String() != "-25000000000000000000" {
		t.Fatalf("signed Scan = %v, %v", s, err)
	}
	for _, src := range []any{1.5, "x"} {
		if err := s.Scan(src); !codeIs(err, errs.CodeDecodeFailed) {
			t.Fatalf("signed Scan %v err = %v", src, err)
		}
	}
	if v, err := s.Value(); err != nil || v != "-25000000000000000000" {
		t.Fatalf("signed Value = %v, %v", v, err)
	}
}

func TestUnitsPostgresRoundTripThroughNumeric78(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	ctx := context.Background()
	if _, err := db.Exec(
		ctx,
		`CREATE TABLE wide_amounts (units numeric(78,0) NOT NULL, delta numeric(78,0) NOT NULL)`,
	); err != nil {
		t.Fatal(err)
	}
	top := maxUnits().String()
	rows := []struct{ units, delta string }{
		{top, "-" + top},
		{"25000000000000000000", "-12000000000000000000"},
		{"18446744073709551616", "9223372036854775808"},
		{"0", "0"},
	}
	for _, r := range rows {
		if _, err := db.Exec(ctx, `DELETE FROM wide_amounts`); err != nil {
			t.Fatal(err)
		}
		units, delta := mustUnits(t, r.units), mustSigned(t, r.delta)
		if _, err := db.Exec(ctx, `INSERT INTO wide_amounts VALUES ($1, $2)`, units, delta); err != nil {
			t.Fatalf("insert %v, %v: %v", units, delta, err)
		}
		var gotUnits money.Units
		var gotDelta money.SignedUnits
		var text string
		if err := db.QueryRow(ctx, `SELECT units, delta, units::text FROM wide_amounts`).
			Scan(&gotUnits, &gotDelta, &text); err != nil {
			t.Fatalf("scan %v, %v: %v", units, delta, err)
		}
		if !gotUnits.Equal(units) || !gotDelta.Equal(delta) || text != r.units {
			t.Fatalf("round trip = %v, %v, stored %q; want %v, %v, %q", gotUnits, gotDelta, text, units, delta, r.units)
		}
	}
}

func drawSigned(t *rapid.T, label string) money.SignedUnits {
	v := anyUnitsBig(t, label)
	if rapid.Bool().Draw(t, label+"Neg") {
		v = new(big.Int).Neg(v)
	}
	s, err := money.SignedUnitsFromBig(v)
	if err != nil {
		t.Skip()
	}
	return s
}

func TestSignedUnitsAddIsExact(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		sa, sb := drawSigned(t, "a"), drawSigned(t, "b")
		want := new(big.Int).Add(sa.Big(), sb.Big())
		got, err := sa.Add(sb)
		if want.BitLen() > 256 {
			if err == nil {
				t.Fatalf("%v + %v = %v, want overflow error", sa, sb, got)
			}
			return
		}
		if err != nil || got.Big().Cmp(want) != 0 {
			t.Fatalf("%v + %v = %v, %v; want %v", sa, sb, got, err, want)
		}
		back, err := got.Add(sb.Neg())
		if err != nil || !back.Equal(sa) {
			t.Fatalf("(%v + %v) - %v = %v, %v", sa, sb, sb, back, err)
		}
	})
}

func codeIs(err error, code errs.Code) bool { return errs.CodeOf(err) == code }
