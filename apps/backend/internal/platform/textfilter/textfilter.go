package textfilter

import (
	_ "embed"
	"log/slog"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

//go:embed blocklist.txt
var blocklistText string

func unleet(r rune) rune {
	switch r {
	case '0':
		return 'o'
	case '1', '!':
		return 'i'
	case '3':
		return 'e'
	case '4', '@':
		return 'a'
	case '5', '$':
		return 's'
	case '7', '+':
		return 't'
	}
	return r
}

func loadBlocked(text string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, line := range strings.Split(text, "\n") {
		if word := strings.TrimSpace(line); word != "" {
			set[collapse(normalize(word))] = struct{}{}
		}
	}
	return set
}

func normalize(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(unleet(r))
	}
	return b.String()
}

func collapse(s string) string {
	var b strings.Builder
	var last rune
	for _, r := range s {
		if r != last {
			b.WriteRune(r)
		}
		last = r
	}
	return b.String()
}

func tokens(s string) []string {
	return strings.FieldsFunc(normalize(s), func(r rune) bool { return !unicode.IsLetter(r) })
}

func Allowed(text string) bool {
	blocked := loadBlocked(blocklistText)
	for _, token := range tokens(text) {
		if _, hit := blocked[collapse(token)]; hit {
			return false
		}
	}
	return true
}

func Check(op, text string) error {
	if Allowed(text) {
		return nil
	}
	return errs.New(errs.CodeContentNotAllowed, op, slog.Int("chars", len(text)))
}
