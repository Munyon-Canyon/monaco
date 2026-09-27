package observability

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

const masked = "***"

var (
	base58SecretKey = regexp.MustCompile(`[1-9A-HJ-NP-Za-km-z]{86,}`)
	byteArrayKey    = regexp.MustCompile(`\[\s*\d{1,3}(\s*,\s*\d{1,3}){63}\s*\]`)
	jwt             = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`)
)

func sensitiveKey(key string) bool {
	switch strings.ToLower(key) {
	case "phone", "email", "token", "key", "seed", "signature", "secret", "authorization":
		return true
	}
	return false
}

func secretValue(s string) bool {
	return base58SecretKey.MatchString(s) || byteArrayKey.MatchString(s) || jwt.MatchString(s)
}

func redact(a slog.Attr) slog.Attr {
	if sensitiveKey(a.Key) {
		return slog.String(a.Key, masked)
	}
	v := a.Value.Resolve()
	switch {
	case v.Kind() == slog.KindGroup:
		v = slog.GroupValue(redactAll(v.Group())...)
	case v.Kind() == slog.KindString && secretValue(v.String()),
		v.Kind() == slog.KindAny && secretValue(fmt.Sprint(v.Any())):
		v = slog.StringValue(masked)
	}
	return slog.Attr{Key: a.Key, Value: v}
}

func redactAll(attrs []slog.Attr) []slog.Attr {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = redact(a)
	}
	return out
}
