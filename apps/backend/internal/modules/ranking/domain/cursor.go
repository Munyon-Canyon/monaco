package domain

import (
	"encoding/base64"
	"log/slog"
	"math"
	"strconv"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	DefaultLimit = 20
	MaxLimit     = 50
)

func EncodeCursor(lastRank int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(lastRank)))
}

func DecodeCursor(raw string) (int, error) {
	const op = "ranking.DecodeCursor"
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	rank, err := strconv.Atoi(string(decoded))
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	if rank < 0 || rank > math.MaxInt32 {
		return 0, errs.New(errs.CodeInvalidInput, op, slog.Int("rank", rank))
	}
	return rank, nil
}

func ParseLimit(raw *int) (int, error) {
	if raw == nil {
		return DefaultLimit, nil
	}
	if *raw < 1 || *raw > MaxLimit {
		return 0, errs.New(errs.CodeInvalidInput, "ranking.ParseLimit", slog.Int("limit", *raw))
	}
	return *raw, nil
}
