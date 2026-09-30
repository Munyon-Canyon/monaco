package cabal_test

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
)

func TestParseName_trimsAndKeepsANameOfThreeToFortyCharacters(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"abc":                         "abc",
		"  Friends pot  ":             "Friends pot",
		"  abc ":                      "abc",
		strings.Repeat("x", 40):       strings.Repeat("x", 40),
		"ééé":                         "ééé",
		strings.Repeat("é", 40):       strings.Repeat("é", 40),
		"a b":                         "a b",
		" " + strings.Repeat("y", 40): strings.Repeat("y", 40),
		"Café ☕ club":                 "Café ☕ club",
	} {
		got, err := domain.ParseName(raw)
		if err != nil || got.String() != want {
			t.Errorf("ParseName(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
}

func TestParseName_refusesTooShortTooLongBlankAndUnprintableNames(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"empty":            "",
		"only spaces":      "     ",
		"two characters":   "ab",
		"two after trim":   "  ab  ",
		"two multibyte":    "éé",
		"forty one":        strings.Repeat("x", 41),
		"forty one runes":  strings.Repeat("é", 41),
		"a null byte":      "ab\x00c",
		"a newline":        "line\nbreak",
		"a tab":            "tab\tbed",
		"invalid utf8":     "\xff\xfe\xfd",
		"invalid utf8 mid": "ab\xffcd",
	} {
		got, err := domain.ParseName(raw)
		wantCode(t, name, err, errs.CodeInvalidInput)
		if got.String() != "" {
			t.Errorf("%s: name = %q, want the zero value", name, got)
		}
	}
}

func wantName(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	chars := utf8.RuneCountInString(trimmed)
	return trimmed, utf8.ValidString(trimmed) && chars >= 3 && chars <= 40 &&
		!strings.ContainsFunc(trimmed, unicode.IsControl)
}

func TestParseName_acceptsExactlyTheTrimmedNamesInRange(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.OneOf(rapid.String(), rapid.StringN(1, 60, -1), rapid.StringOfN(rapid.RuneFrom([]rune{'a', 'é', ' ', '\n'}), 0, 50, -1)).
			Draw(t, "raw")
		want, ok := wantName(raw)
		got, err := domain.ParseName(raw)
		if !ok {
			if errs.CodeOf(err) != errs.CodeInvalidInput || err == nil {
				t.Fatalf("ParseName(%q) = %q, %v; want invalid_input", raw, got, err)
			}
			return
		}
		again, errAgain := domain.ParseName(got.String())
		if err != nil || got.String() != want || errAgain != nil || again != got {
			t.Fatalf(
				"ParseName(%q) = %q, %v and again %q, %v; want %q both times",
				raw,
				got,
				err,
				again,
				errAgain,
				want,
			)
		}
	})
}
