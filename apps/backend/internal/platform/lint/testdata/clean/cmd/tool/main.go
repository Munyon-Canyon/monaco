package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

func main() {
	done := make(chan struct{})
	go func() { close(done) }()
	<-done
	boundary.Fail(context.Background(), slog.New(slog.NewTextHandler(os.Stderr, nil)), context.Canceled)
}
