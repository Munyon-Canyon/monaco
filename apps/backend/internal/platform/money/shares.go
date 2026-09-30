package money

import (
	"database/sql/driver"
	"log/slog"
	"math/bits"
	"strconv"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type SharesUnits struct{ v uint64 }

func SharesUnitsFromUint64(v uint64) SharesUnits { return SharesUnits{v: v} }

func ParseSharesUnits(raw string) (SharesUnits, error) {
	v, ok := parseUnsigned(raw)
	if !ok {
		return SharesUnits{}, errs.New(errs.CodeInvalidInput, "money.ParseSharesUnits", slog.String("raw", raw))
	}
	return SharesUnits{v: v}, nil
}

func (s SharesUnits) Add(o SharesUnits) (SharesUnits, error) {
	sum, carry := bits.Add64(s.v, o.v, 0)
	if carry != 0 {
		return SharesUnits{}, errs.New(
			errs.CodeInvalidInput,
			"money.SharesUnits.Add",
			slog.Uint64("a", s.v),
			slog.Uint64("b", o.v),
		)
	}
	return SharesUnits{v: sum}, nil
}

func (s SharesUnits) Sub(o SharesUnits) (SharesUnits, error) {
	if o.v > s.v {
		return SharesUnits{}, errs.New(
			errs.CodeInvalidInput,
			"money.SharesUnits.Sub",
			slog.Uint64("a", s.v),
			slog.Uint64("b", o.v),
		)
	}
	return SharesUnits{v: s.v - o.v}, nil
}

func (s SharesUnits) Uint64() uint64 { return s.v }

func (s SharesUnits) IsZero() bool { return s.v == 0 }

func (s SharesUnits) String() string { return strconv.FormatUint(s.v, 10) }

func (s SharesUnits) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

func (s *SharesUnits) UnmarshalText(text []byte) error {
	v, err := ParseSharesUnits(string(text))
	if err != nil {
		return err
	}
	*s = v
	return nil
}

func (s SharesUnits) Value() (driver.Value, error) { return s.String(), nil }

func (s *SharesUnits) Scan(src any) error {
	raw, err := scanText("money.SharesUnits.Scan", src)
	if err != nil {
		return err
	}
	v, ok := parseUnsigned(raw)
	if !ok {
		return errs.New(errs.CodeDecodeFailed, "money.SharesUnits.Scan", slog.String("raw", raw))
	}
	*s = SharesUnits{v: v}
	return nil
}
