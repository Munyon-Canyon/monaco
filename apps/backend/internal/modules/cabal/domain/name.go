package domain

import (
	"log/slog"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	MinNameLen = 3
	MaxNameLen = 40
)

type Name struct{ value string }

func ParseName(raw string) (Name, error) {
	trimmed := strings.TrimSpace(raw)
	chars := utf8.RuneCountInString(trimmed)
	if chars < MinNameLen || chars > MaxNameLen || !utf8.ValidString(trimmed) ||
		strings.ContainsFunc(trimmed, unicode.IsControl) {
		return Name{}, errs.New(errs.CodeInvalidInput, "cabal.ParseName", slog.Int("chars", chars))
	}
	return Name{value: trimmed}, nil
}

func (n Name) String() string { return n.value }
