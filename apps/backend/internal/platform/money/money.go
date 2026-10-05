package money

import (
	"cmp"
	"log/slog"
	"math/big"
	"math/bits"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Micros struct{ v uint64 }

func MicrosFromUint64(v uint64) Micros { return Micros{v: v} }

func ParseMicros(raw string) (Micros, error) {
	v, ok := parseUnsigned(raw)
	if !ok {
		return Micros{}, errs.New(errs.CodeInvalidInput, "money.ParseMicros", slog.String("raw", raw))
	}
	return Micros{v: v}, nil
}

func (m Micros) Add(o Micros) (Micros, error) {
	sum, carry := bits.Add64(m.v, o.v, 0)
	if carry != 0 {
		return Micros{}, errs.New(
			errs.CodeInvalidInput,
			"money.Micros.Add",
			slog.Uint64("a", m.v),
			slog.Uint64("b", o.v),
		)
	}
	return Micros{v: sum}, nil
}

func (m Micros) Sub(o Micros) (Micros, error) {
	if o.v > m.v {
		return Micros{}, errs.New(
			errs.CodeInvalidInput,
			"money.Micros.Sub",
			slog.Uint64("a", m.v),
			slog.Uint64("b", o.v),
		)
	}
	return Micros{v: m.v - o.v}, nil
}

func (m Micros) Delta(o Micros) (SignedMicros, error) {
	d := new(big.Int).Sub(new(big.Int).SetUint64(m.v), new(big.Int).SetUint64(o.v))
	if !d.IsInt64() {
		return SignedMicros{}, errs.New(
			errs.CodeInvalidInput,
			"money.Micros.Delta",
			slog.Uint64("a", m.v),
			slog.Uint64("b", o.v),
		)
	}
	return SignedMicros{v: d.Int64()}, nil
}

func (m Micros) Uint64() uint64 { return m.v }

func (m Micros) Cmp(o Micros) int { return cmp.Compare(m.v, o.v) }

func (m Micros) IsZero() bool { return m.v == 0 }

func (m Micros) String() string { return strconv.FormatUint(m.v, 10) }

type SignedMicros struct{ v int64 }

func SignedMicrosFromInt64(v int64) SignedMicros { return SignedMicros{v: v} }

func ParseSignedMicros(raw string) (SignedMicros, error) {
	v, ok := parseSigned(raw)
	if !ok {
		return SignedMicros{}, errs.New(errs.CodeInvalidInput, "money.ParseSignedMicros", slog.String("raw", raw))
	}
	return SignedMicros{v: v}, nil
}

func (s SignedMicros) Micros() (Micros, error) {
	if s.v < 0 {
		return Micros{}, errs.New(errs.CodeInvalidInput, "money.SignedMicros.Micros", slog.Int64("v", s.v))
	}
	return Micros{v: uint64(s.v)}, nil
}

func (s SignedMicros) Int64() int64 { return s.v }

func (s SignedMicros) Cmp(o SignedMicros) int { return cmp.Compare(s.v, o.v) }

func (s SignedMicros) IsZero() bool { return s.v == 0 }

func (s SignedMicros) String() string { return strconv.FormatInt(s.v, 10) }

type BaseUnits struct {
	v        uint64
	decimals uint8
}

func NewBaseUnits(v uint64, decimals uint8) BaseUnits { return BaseUnits{v: v, decimals: decimals} }

func (b BaseUnits) Add(o BaseUnits) (BaseUnits, error) {
	if b.decimals != o.decimals {
		return BaseUnits{}, decimalsMismatch("money.BaseUnits.Add", b, o)
	}
	sum, carry := bits.Add64(b.v, o.v, 0)
	if carry != 0 {
		return BaseUnits{}, errs.New(
			errs.CodeInvalidInput,
			"money.BaseUnits.Add",
			slog.Uint64("a", b.v),
			slog.Uint64("b", o.v),
		)
	}
	return BaseUnits{v: sum, decimals: b.decimals}, nil
}

func (b BaseUnits) Sub(o BaseUnits) (BaseUnits, error) {
	if b.decimals != o.decimals {
		return BaseUnits{}, decimalsMismatch("money.BaseUnits.Sub", b, o)
	}
	if o.v > b.v {
		return BaseUnits{}, errs.New(
			errs.CodeInvalidInput,
			"money.BaseUnits.Sub",
			slog.Uint64("a", b.v),
			slog.Uint64("b", o.v),
		)
	}
	return BaseUnits{v: b.v - o.v, decimals: b.decimals}, nil
}

func (b BaseUnits) Cmp(o BaseUnits) (int, error) {
	if b.decimals != o.decimals {
		return 0, decimalsMismatch("money.BaseUnits.Cmp", b, o)
	}
	return cmp.Compare(b.v, o.v), nil
}

func (b BaseUnits) Uint64() uint64 { return b.v }

func (b BaseUnits) Decimals() uint8 { return b.decimals }

func (b BaseUnits) IsZero() bool { return b.v == 0 }

func (b BaseUnits) String() string { return strconv.FormatUint(b.v, 10) }

func MulDiv(a, b, c uint64) (uint64, error) {
	if c == 0 {
		return 0, errs.New(errs.CodeInvalidInput, "money.MulDiv", slog.Uint64("a", a), slog.Uint64("b", b))
	}
	q := new(big.Int).Mul(new(big.Int).SetUint64(a), new(big.Int).SetUint64(b))
	q.Quo(q, new(big.Int).SetUint64(c))
	if !q.IsUint64() {
		return 0, errs.New(errs.CodeInvalidInput, "money.MulDiv",
			slog.Uint64("a", a), slog.Uint64("b", b), slog.Uint64("c", c))
	}
	return q.Uint64(), nil
}

func MulDivCeil(a, b, c uint64) (uint64, error) {
	if c == 0 {
		return 0, errs.New(errs.CodeInvalidInput, "money.MulDivCeil", slog.Uint64("a", a), slog.Uint64("b", b))
	}
	q := new(big.Int).Mul(new(big.Int).SetUint64(a), new(big.Int).SetUint64(b))
	divisor := new(big.Int).SetUint64(c)
	q.Add(q, new(big.Int).Sub(divisor, big.NewInt(1)))
	q.Quo(q, divisor)
	if !q.IsUint64() {
		return 0, errs.New(errs.CodeInvalidInput, "money.MulDivCeil",
			slog.Uint64("a", a), slog.Uint64("b", b), slog.Uint64("c", c))
	}
	return q.Uint64(), nil
}

func decimalsMismatch(op string, a, b BaseUnits) error {
	return errs.New(errs.CodeInvalidInput, op,
		slog.Int("a_decimals", int(a.decimals)), slog.Int("b_decimals", int(b.decimals)))
}

func parseUnsigned(raw string) (uint64, bool) {
	if !canonicalDigits(raw) {
		return 0, false
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	return v, err == nil
}

func parseSigned(raw string) (int64, bool) {
	if raw == "-0" || !canonicalDigits(strings.TrimPrefix(raw, "-")) {
		return 0, false
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	return v, err == nil
}

func canonicalDigits(s string) bool {
	return !strings.HasPrefix(s, "+") && (s == "0" || !strings.HasPrefix(s, "0"))
}
