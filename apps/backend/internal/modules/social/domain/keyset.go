package domain

import (
	"encoding/base64"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Keyset struct {
	Ranked bool
	Count  int32
	At     time.Time
	ID     uuid.UUID
}

func (k Keyset) Encode() string {
	raw := strconv.FormatInt(k.At.UnixMicro(), 10) + "." + k.ID.String()
	if k.Ranked {
		raw = strconv.FormatInt(int64(k.Count), 10) + "." + raw
	}
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func ParseKeyset(raw string) (Keyset, error) {
	const op = "social.ParseKeyset"
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return Keyset{}, invalidCursor(op, raw)
	}
	return parsePosition(string(body), op, raw)
}

func ParseRankedKeyset(raw string) (Keyset, error) {
	const op = "social.ParseRankedKeyset"
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return Keyset{}, invalidCursor(op, raw)
	}
	count, rest, ok := strings.Cut(string(body), ".")
	if !ok {
		return Keyset{}, invalidCursor(op, raw)
	}
	n, err := strconv.ParseInt(count, 10, 32)
	if err != nil {
		return Keyset{}, invalidCursor(op, raw)
	}
	k, err := parsePosition(rest, op, raw)
	k.Ranked, k.Count = true, int32(n)
	return k, err
}

func parsePosition(body, op, raw string) (Keyset, error) {
	micros, id, ok := strings.Cut(body, ".")
	if !ok {
		return Keyset{}, invalidCursor(op, raw)
	}
	at, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return Keyset{}, invalidCursor(op, raw)
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return Keyset{}, invalidCursor(op, raw)
	}
	return Keyset{At: time.UnixMicro(at).UTC(), ID: parsed}, nil
}

func invalidCursor(op, raw string) error {
	return errs.New(errs.CodeInvalidInput, op, slog.String("cursor", raw))
}
