package money

import (
	"log/slog"
	"math/big"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const maxUnitsBits = 256

type Units struct{ v *big.Int }

func UnitsFromUint64(v uint64) Units { return Units{v: new(big.Int).SetUint64(v)} }

func UnitsFromBig(v *big.Int) (Units, error) {
	if v == nil || v.Sign() < 0 || v.BitLen() > maxUnitsBits {
		return Units{}, errs.New(errs.CodeInvalidInput, "money.UnitsFromBig", slog.String("v", bigText(v)))
	}
	return Units{v: new(big.Int).Set(v)}, nil
}

func ParseUnits(raw string) (Units, error) {
	v, ok := parseBig(raw)
	if !ok || v.Sign() < 0 || v.BitLen() > maxUnitsBits {
		return Units{}, errs.New(errs.CodeInvalidInput, "money.ParseUnits", slog.String("raw", raw))
	}
	return Units{v: v}, nil
}

func (u Units) big() *big.Int {
	if u.v == nil {
		return new(big.Int)
	}
	return u.v
}

func (u Units) Big() *big.Int { return new(big.Int).Set(u.big()) }

func (u Units) Add(o Units) (Units, error) {
	return unitsFromSum("money.Units.Add", new(big.Int).Add(u.big(), o.big()), u, o)
}

func (u Units) Sub(o Units) (Units, error) {
	d := new(big.Int).Sub(u.big(), o.big())
	if d.Sign() < 0 {
		return Units{}, errs.New(errs.CodeInvalidInput, "money.Units.Sub",
			slog.String("a", u.String()), slog.String("b", o.String()))
	}
	return Units{v: d}, nil
}

func (u Units) Cmp(o Units) int { return u.big().Cmp(o.big()) }

func (u Units) Equal(o Units) bool { return u.Cmp(o) == 0 }

func (u Units) IsZero() bool { return u.big().Sign() == 0 }

func (u Units) IsUint64() bool { return u.big().IsUint64() }

func (u Units) Uint64() (uint64, error) {
	if !u.big().IsUint64() {
		return 0, errs.New(errs.CodeInvalidInput, "money.Units.Uint64", slog.String("v", u.String()))
	}
	return u.big().Uint64(), nil
}

func (u Units) Int64() (int64, error) {
	if !u.big().IsInt64() {
		return 0, errs.New(errs.CodeInvalidInput, "money.Units.Int64", slog.String("v", u.String()))
	}
	return u.big().Int64(), nil
}

func (u Units) Int64OrZero() int64 {
	if !u.big().IsInt64() {
		return 0
	}
	return u.big().Int64()
}

func (u Units) Signed() SignedUnits { return SignedUnits{v: u.Big()} }

func (u Units) Micros() (Micros, error) {
	v, err := u.Uint64()
	if err != nil {
		return Micros{}, errs.Wrap(err, errs.CodeInvalidInput, "money.Units.Micros")
	}
	return MicrosFromUint64(v), nil
}

func (u Units) String() string { return u.big().String() }

func UnitsFromMicros(m Micros) Units { return UnitsFromUint64(m.Uint64()) }

func UnitsMulDiv(a, b, c Units) (Units, error) {
	if c.IsZero() {
		return Units{}, errs.New(errs.CodeInvalidInput, "money.UnitsMulDiv",
			slog.String("a", a.String()), slog.String("b", b.String()))
	}
	q := new(big.Int).Mul(a.big(), b.big())
	q.Quo(q, c.big())
	return unitsFromSum("money.UnitsMulDiv", q, a, b)
}

type SignedUnits struct{ v *big.Int }

func SignedUnitsFromInt64(v int64) SignedUnits { return SignedUnits{v: big.NewInt(v)} }

func SignedUnitsFromBig(v *big.Int) (SignedUnits, error) {
	if v == nil || v.BitLen() > maxUnitsBits {
		return SignedUnits{}, errs.New(errs.CodeInvalidInput, "money.SignedUnitsFromBig", slog.String("v", bigText(v)))
	}
	return SignedUnits{v: new(big.Int).Set(v)}, nil
}

func ParseSignedUnits(raw string) (SignedUnits, error) {
	v, ok := parseBig(raw)
	if !ok || v.BitLen() > maxUnitsBits {
		return SignedUnits{}, errs.New(errs.CodeInvalidInput, "money.ParseSignedUnits", slog.String("raw", raw))
	}
	return SignedUnits{v: v}, nil
}

func (s SignedUnits) big() *big.Int {
	if s.v == nil {
		return new(big.Int)
	}
	return s.v
}

func (s SignedUnits) Big() *big.Int { return new(big.Int).Set(s.big()) }

func (s SignedUnits) Add(o SignedUnits) (SignedUnits, error) {
	sum := new(big.Int).Add(s.big(), o.big())
	if sum.BitLen() > maxUnitsBits {
		return SignedUnits{}, errs.New(errs.CodeInvalidInput, "money.SignedUnits.Add",
			slog.String("a", s.String()), slog.String("b", o.String()))
	}
	return SignedUnits{v: sum}, nil
}

func (s SignedUnits) Neg() SignedUnits { return SignedUnits{v: new(big.Int).Neg(s.big())} }

func (s SignedUnits) Abs() Units { return Units{v: new(big.Int).Abs(s.big())} }

func (s SignedUnits) Sign() int { return s.big().Sign() }

func (s SignedUnits) IsZero() bool { return s.big().Sign() == 0 }

func (s SignedUnits) Cmp(o SignedUnits) int { return s.big().Cmp(o.big()) }

func (s SignedUnits) Equal(o SignedUnits) bool { return s.Cmp(o) == 0 }

func (s SignedUnits) Units() (Units, error) {
	if s.big().Sign() < 0 {
		return Units{}, errs.New(errs.CodeInvalidInput, "money.SignedUnits.Units", slog.String("v", s.String()))
	}
	return Units{v: s.Big()}, nil
}

func (s SignedUnits) Int64() (int64, error) {
	if !s.big().IsInt64() {
		return 0, errs.New(errs.CodeInvalidInput, "money.SignedUnits.Int64", slog.String("v", s.String()))
	}
	return s.big().Int64(), nil
}

func (s SignedUnits) Int64OrZero() int64 {
	if !s.big().IsInt64() {
		return 0
	}
	return s.big().Int64()
}

func (s SignedUnits) String() string { return s.big().String() }

func SignedUnitsFromMicros(m SignedMicros) SignedUnits { return SignedUnitsFromInt64(m.Int64()) }

func unitsFromSum(op string, v *big.Int, a, b Units) (Units, error) {
	if v.BitLen() > maxUnitsBits {
		return Units{}, errs.New(errs.CodeInvalidInput, op, slog.String("a", a.String()), slog.String("b", b.String()))
	}
	return Units{v: v}, nil
}

func parseBig(raw string) (*big.Int, bool) {
	if raw == "-0" || !canonicalDigits(strings.TrimPrefix(raw, "-")) || !allDigits(strings.TrimPrefix(raw, "-")) {
		return nil, false
	}
	v, ok := new(big.Int).SetString(raw, 10)
	return v, ok
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func bigText(v *big.Int) string {
	if v == nil {
		return "nil"
	}
	return v.String()
}

func PricePerUnits(usdc Micros, tokens Units, decimals uint8) Micros {
	if tokens.IsZero() {
		return Micros{}
	}
	unit, err := UnitsFromBig(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	if err != nil {
		return Micros{}
	}
	price, err := UnitsMulDiv(UnitsFromMicros(usdc), unit, tokens)
	if err != nil {
		return Micros{}
	}
	micros, err := price.Micros()
	if err != nil {
		return Micros{}
	}
	return micros
}
