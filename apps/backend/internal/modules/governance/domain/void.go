package domain

import (
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const MaxVoidReasonRunes = 500

type VoidReason string

func ParseVoidReason(raw string) (VoidReason, error) {
	n := utf8.RuneCountInString(raw)
	if strings.TrimSpace(raw) == "" || n > MaxVoidReasonRunes || !utf8.ValidString(raw) {
		return "", errs.New(errs.CodeInvalidInput, "governance.ParseVoidReason", slog.Int("runes", n))
	}
	return VoidReason(raw), nil
}
