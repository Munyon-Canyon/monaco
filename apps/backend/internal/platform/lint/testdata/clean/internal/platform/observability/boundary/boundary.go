package boundary

import (
	"context"
	"log/slog"
)

func Fail(ctx context.Context, logger *slog.Logger, err error) {
	logger.ErrorContext(ctx, "request failed", slog.Any("err", err))
}
