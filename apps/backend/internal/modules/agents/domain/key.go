package domain

import (
	"crypto/sha256"
	"io"
	"log/slog"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	KeyPrefix   = "monaco_ak_"
	KeyAlphabet = "23456789abcdefghjkmnpqrstuvwxyz"
	KeyLength   = 32
	acceptBelow = 256 - 256%len(KeyAlphabet)
)

type Key struct {
	value string
}

func MintKey(random io.Reader) (Key, error) {
	out := make([]byte, 0, KeyLength)
	buf := make([]byte, KeyLength)
	for len(out) < KeyLength {
		if _, err := io.ReadFull(random, buf); err != nil {
			return Key{}, errs.Wrap(err, errs.CodeInternal, "agents.MintKey")
		}
		for _, b := range buf {
			if int(b) < acceptBelow && len(out) < KeyLength {
				out = append(out, KeyAlphabet[int(b)%len(KeyAlphabet)])
			}
		}
	}
	return Key{value: KeyPrefix + string(out)}, nil
}

func ParseKey(raw string) (Key, error) {
	body, ok := strings.CutPrefix(raw, KeyPrefix)
	if !ok || len(body) != KeyLength || strings.Trim(body, KeyAlphabet) != "" {
		return Key{}, errs.New(errs.CodeInvalidInput, "agents.ParseKey", slog.Int("len", len(raw)))
	}
	return Key{value: raw}, nil
}

func (k Key) String() string { return k.value }

func (k Key) Hash() []byte {
	sum := sha256.Sum256([]byte(k.value))
	return sum[:]
}

func (Key) LogValue() slog.Value { return slog.StringValue("[redacted]") }
