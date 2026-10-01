package domain

import (
	"log/slog"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	MinHandleLen = 3
	MaxHandleLen = 20

	HandleChangeInterval = 30 * 24 * time.Hour
)

type Handle struct{ name string }

func ParseHandle(raw string) (Handle, error) {
	if len(raw) < MinHandleLen || len(raw) > MaxHandleLen || strings.IndexFunc(raw, notHandleRune) >= 0 {
		return Handle{}, errs.New(errs.CodeHandleInvalid, "identity.ParseHandle", slog.Int("bytes", len(raw)))
	}
	return Handle{name: strings.ToLower(raw)}, nil
}

func notHandleRune(r rune) bool {
	return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_'
}

func (h Handle) String() string { return h.name }
