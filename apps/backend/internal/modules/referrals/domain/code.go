package domain

import (
	"io"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	CodeAlphabet = "23456789abcdefghjkmnpqrstuvwxyz"
	CodeLength   = 8
	acceptBelow  = 256 - 256%len(CodeAlphabet)
)

type Code string

func NewRandomCode(random io.Reader) (Code, error) {
	out := make([]byte, 0, CodeLength)
	buf := make([]byte, CodeLength)
	for len(out) < CodeLength {
		if _, err := io.ReadFull(random, buf); err != nil {
			return "", errs.Wrap(err, errs.CodeInternal, "referrals.NewRandomCode")
		}
		for _, b := range buf {
			if int(b) < acceptBelow && len(out) < CodeLength {
				out = append(out, CodeAlphabet[int(b)%len(CodeAlphabet)])
			}
		}
	}
	return Code(out), nil
}

func NormalizeInput(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func IsRandomShape(s string) bool {
	return len(s) == CodeLength && strings.Trim(s, CodeAlphabet) == ""
}
