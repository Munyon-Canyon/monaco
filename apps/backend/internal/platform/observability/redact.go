package observability

import (
	"fmt"
	"log/slog"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

const (
	masked       = "***"
	keyBytes     = 64
	maxRedactDep = 8
)

var (
	base58SecretKey = regexp.MustCompile(`[1-9A-HJ-NP-Za-km-z]{86,}`)
	base64SecretKey = regexp.MustCompile(`[A-Za-z0-9+/]{86}==`)
	byteArrayKey    = regexp.MustCompile(`\[\s*\d{1,3}(\s*,\s*\d{1,3}){63}\s*\]`)
	jwt             = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`)
	bearerToken     = regexp.MustCompile(`(?i)\bbearer\s+\S+`)
	sensitiveQuery  = regexp.MustCompile(
		`(?i)([?&][^=&#\s]*(phone|email|token|key|seed|signature|secret|authorization|password|mnemonic)[^=&#\s]*=)[^&#\s]*`,
	)
)

func sensitiveKey(key string) bool {
	for _, word := range keyWords(key) {
		switch strings.TrimSuffix(word, "s") {
		case "phone", "email", "token", "key", "seed", "signature", "secret", "authorization", "password", "mnemonic":
			return true
		}
	}
	return false
}

func keyWords(key string) []string {
	var words []string
	var cur []rune
	prevLower := false
	for _, r := range key {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) || unicode.IsUpper(r) && prevLower {
			words, cur = appendWord(words, cur), cur[:0]
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur = append(cur, unicode.ToLower(r))
		}
		prevLower = unicode.IsLower(r)
	}
	return appendWord(words, cur)
}

func appendWord(words []string, cur []rune) []string {
	if len(cur) == 0 {
		return words
	}
	return append(words, string(cur))
}

func redactString(s string) string {
	for _, p := range []*regexp.Regexp{base58SecretKey, base64SecretKey, byteArrayKey, jwt, bearerToken} {
		if p.MatchString(s) {
			return masked
		}
	}
	return sensitiveQuery.ReplaceAllString(s, "${1}"+masked)
}

func redact(a slog.Attr) slog.Attr {
	return redactDepth(a, 0)
}

func redactDepth(a slog.Attr, depth int) slog.Attr {
	if sensitiveKey(a.Key) || depth > maxRedactDep {
		return slog.String(a.Key, masked)
	}
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindGroup:
		attrs := v.Group()
		out := make([]slog.Attr, len(attrs))
		for i, g := range attrs {
			out[i] = redactDepth(g, depth+1)
		}
		v = slog.GroupValue(out...)
	case slog.KindString:
		v = slog.StringValue(redactString(v.String()))
	case slog.KindAny:
		v = redactAny(v.Any(), depth)
	case slog.KindBool, slog.KindDuration, slog.KindFloat64, slog.KindInt64, slog.KindTime, slog.KindUint64,
		slog.KindLogValuer:
	}
	return slog.Attr{Key: a.Key, Value: v}
}

func redactAny(x any, depth int) slog.Value {
	rv := reflect.ValueOf(x)
	if isKeyBytes(rv) {
		return slog.StringValue(masked)
	}
	switch x.(type) {
	case error, fmt.Stringer:
		return redactFormatted(x)
	}
	for rv.Kind() == reflect.Pointer && !rv.IsNil() {
		rv = rv.Elem()
	}
	switch {
	case rv.Kind() == reflect.Struct:
		return slog.GroupValue(structAttrs(rv, depth)...)
	case rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String:
		return slog.GroupValue(mapAttrs(rv, depth)...)
	}
	return redactFormatted(x)
}

func redactFormatted(x any) slog.Value {
	s := fmt.Sprint(x)
	if r := redactString(s); r != s {
		return slog.StringValue(r)
	}
	return slog.AnyValue(x)
}

func isKeyBytes(rv reflect.Value) bool {
	k := rv.Kind()
	return (k == reflect.Slice || k == reflect.Array) && rv.Type().Elem().Kind() == reflect.Uint8 &&
		rv.Len() == keyBytes
}

func structAttrs(rv reflect.Value, depth int) []slog.Attr {
	t := rv.Type()
	var out []slog.Attr
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name := f.Name
		if tag, _, _ := strings.Cut(f.Tag.Get("json"), ","); tag != "" {
			name = tag
		}
		out = append(out, redactDepth(slog.Any(name, rv.Field(i).Interface()), depth+1))
	}
	return out
}

func mapAttrs(rv reflect.Value, depth int) []slog.Attr {
	out := make([]slog.Attr, 0, rv.Len())
	iter := rv.MapRange()
	for iter.Next() {
		out = append(out, redactDepth(slog.Any(iter.Key().String(), iter.Value().Interface()), depth+1))
	}
	slices.SortFunc(out, func(a, b slog.Attr) int { return strings.Compare(a.Key, b.Key) })
	return out
}

func redactAll(attrs []slog.Attr) []slog.Attr {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = redact(a)
	}
	return out
}
