package domain

import (
	"io"
	"log/slog"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	inviteCodeLen     = 10
)

type InviteCode struct{ value string }

func NewInviteCode(random io.Reader) (InviteCode, error) {
	raw := make([]byte, inviteCodeLen)
	if _, err := io.ReadFull(random, raw); err != nil {
		return InviteCode{}, errs.Wrap(err, errs.CodeInternal, "cabal.NewInviteCode")
	}
	for i, b := range raw {
		raw[i] = crockfordAlphabet[b%byte(len(crockfordAlphabet))]
	}
	return InviteCode{value: string(raw)}, nil
}

func ParseInviteCode(raw string) (InviteCode, error) {
	code := strings.Map(crockfordDigit, strings.ToUpper(strings.TrimSpace(raw)))
	if len(code) != inviteCodeLen || strings.IndexFunc(code, notCrockford) >= 0 {
		return InviteCode{}, errs.New(errs.CodeInvalidInput, "cabal.ParseInviteCode", slog.Int("bytes", len(raw)))
	}
	return InviteCode{value: code}, nil
}

func crockfordDigit(r rune) rune {
	switch r {
	case 'I', 'L':
		return '1'
	case 'O':
		return '0'
	}
	return r
}

func notCrockford(r rune) bool { return !strings.ContainsRune(crockfordAlphabet, r) }

func (c InviteCode) String() string { return c.value }
