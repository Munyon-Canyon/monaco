package app

import (
	"context"
	"log/slog"
)

func Report(ctx context.Context, logger *slog.Logger) {
	logger.ErrorContext(ctx, "failed")
}
