package money

import (
	"database/sql/driver"
	"fmt"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func (m Micros) MarshalText() ([]byte, error) { return []byte(m.String()), nil }

func (m *Micros) UnmarshalText(text []byte) error {
	v, err := ParseMicros(string(text))
	if err != nil {
		return err
	}
	*m = v
	return nil
}

func (m Micros) Value() (driver.Value, error) { return m.String(), nil }

func (m *Micros) Scan(src any) error {
	raw, err := scanText("money.Micros.Scan", src)
	if err != nil {
		return err
	}
	v, ok := parseUnsigned(raw)
	if !ok {
		return errs.New(errs.CodeDecodeFailed, "money.Micros.Scan", slog.String("raw", raw))
	}
	*m = Micros{v: v}
	return nil
}

func (s SignedMicros) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

func (s *SignedMicros) UnmarshalText(text []byte) error {
	v, err := ParseSignedMicros(string(text))
	if err != nil {
		return err
	}
	*s = v
	return nil
}

func (s SignedMicros) Value() (driver.Value, error) { return s.String(), nil }

func (s *SignedMicros) Scan(src any) error {
	raw, err := scanText("money.SignedMicros.Scan", src)
	if err != nil {
		return err
	}
	v, ok := parseSigned(raw)
	if !ok {
		return errs.New(errs.CodeDecodeFailed, "money.SignedMicros.Scan", slog.String("raw", raw))
	}
	*s = SignedMicros{v: v}
	return nil
}

func scanText(op string, src any) (string, error) {
	switch s := src.(type) {
	case string:
		return s, nil
	case []byte:
		return string(s), nil
	}
	return "", errs.New(errs.CodeDecodeFailed, op, slog.String("type", fmt.Sprintf("%T", src)))
}
