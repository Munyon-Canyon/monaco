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
	At time.Time
	ID uuid.UUID
}

func (k Keyset) Encode() string {
	raw := strconv.FormatInt(k.At.UnixMicro(), 10) + "." + k.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func ParseKeyset(raw string) (Keyset, error) {
	const op = "social.ParseKeyset"
	invalid := errs.New(errs.CodeInvalidInput, op, slog.String("cursor", raw))
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return Keyset{}, invalid
	}
	micros, id, ok := strings.Cut(string(body), ".")
	if !ok {
		return Keyset{}, invalid
	}
	at, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return Keyset{}, invalid
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return Keyset{}, invalid
	}
	return Keyset{At: time.UnixMicro(at).UTC(), ID: parsed}, nil
}
