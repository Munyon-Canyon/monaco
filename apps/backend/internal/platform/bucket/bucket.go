package bucket

import (
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Size string

const (
	Day  Size = "day"
	Week Size = "week"
)

func Parse(raw string) (Size, error) {
	switch s := Size(raw); s {
	case Day, Week:
		return s, nil
	}
	return "", errs.New(errs.CodeInvalidInput, "bucket.Parse", slog.String("bucket", raw))
}
