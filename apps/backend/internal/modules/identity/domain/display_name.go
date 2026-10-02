package domain

import (
	"log/slog"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type DisplayName struct{ value string }

func ParseDisplayName(raw string) (DisplayName, error) {
	if !utf8.ValidString(raw) {
		return DisplayName{}, invalidDisplayName("invalid_characters")
	}
	value, reason := normalizeDisplayName(strings.TrimSpace(norm.NFC.String(raw)))
	if reason != "" {
		return DisplayName{}, invalidDisplayName(reason)
	}
	return DisplayName{value: value}, nil
}

func normalizeDisplayName(name string) (string, string) {
	n := displayNameNormalizer{}
	for _, r := range name {
		if reason := n.add(r); reason != "" {
			return "", reason
		}
	}
	value := n.out.String()
	switch count := utf8.RuneCountInString(value); {
	case count == 0:
		return "", "required"
	case count > 32:
		return "", "too_long"
	case !n.word:
		return "", "needs_letter_or_number"
	}
	return value, ""
}

type displayNameNormalizer struct {
	out   strings.Builder
	space bool
	marks int
	word  bool
}

func (n *displayNameNormalizer) add(r rune) string {
	if r == ' ' || unicode.Is(unicode.Zs, r) {
		n.space, n.marks = n.out.Len() > 0, 0
		return ""
	}
	if !allowedDisplayNameRune(r) {
		return "invalid_characters"
	}
	if unicode.IsMark(r) {
		n.marks++
		if n.marks > 2 || n.out.Len() == 0 {
			return "invalid_characters"
		}
	} else {
		n.marks = 0
	}
	if n.space {
		n.out.WriteByte(' ')
		n.space = false
	}
	n.word = n.word || unicode.IsLetter(r) || unicode.IsNumber(r)
	n.out.WriteRune(r)
	return ""
}

func (n DisplayName) String() string { return n.value }

func invalidDisplayName(reason string) error {
	return errs.New(errs.CodeDisplayNameInvalid, "identity.ParseDisplayName", slog.String("reason", reason))
}

func allowedDisplayNameRune(r rune) bool {
	switch r {
	case '\u115F', '\u1160', '\u3164', '\uFFA0', '\u2800', '\u034F', '\u17B4', '\u17B5':
		return false
	}
	return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
}
