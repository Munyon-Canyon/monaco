package domain

import (
	"log/slog"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const MaxBioScalars = 160

type Bio struct{ value string }

func ParseBio(raw string) (Bio, error) {
	if !utf8.ValidString(raw) {
		return Bio{}, invalidBio("invalid_characters")
	}
	value := strings.TrimSpace(norm.NFC.String(raw))
	for _, r := range value {
		if r != '\n' && unicode.IsControl(r) {
			return Bio{}, invalidBio("invalid_characters")
		}
	}
	if utf8.RuneCountInString(value) > MaxBioScalars {
		return Bio{}, invalidBio("too_long")
	}
	return Bio{value: value}, nil
}

func (b Bio) String() string { return b.value }

func (b Bio) Empty() bool { return b.value == "" }

func invalidBio(reason string) error {
	return errs.New(errs.CodeInvalidInput, "identity.ParseBio", slog.String("reason", reason))
}
